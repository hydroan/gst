package authz_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

// TestReconcilePoliciesDetectsDrift covers what the report is for: a stored
// rule that no record derives, and a record whose rule is missing.
func TestReconcilePoliciesDetectsDrift(t *testing.T) {
	ctx := context.Background()

	t.Run("in sync when nothing was touched behind the framework", func(t *testing.T) {
		report, err := authz.ReconcilePolicies(ctx)
		require.NoError(t, err)
		require.True(t, report.InSync(), "unexpected drift: %+v", report.Drifts)
	})

	t.Run("reports a permission no record derives", func(t *testing.T) {
		role := util.UUID()
		object := "/api/reconcile/" + role
		require.NoError(t, rbac.RBAC().SetRolePermissions(ctx, tenant.Default, role, []types.Permission{
			{Object: object, Action: "GET"},
		}))
		t.Cleanup(func() { _ = rbac.RBAC().RevokeRolePermissions(ctx, tenant.Default, role) })

		report, err := authz.ReconcilePolicies(ctx)
		require.NoError(t, err)
		require.Contains(t, report.Drifts, authz.PolicyDrift{
			Kind: "permission", Direction: "orphaned",
			Tenant: tenant.Default, Role: role, Object: object, Action: "GET",
		}, "a permission granted to a role that does not exist should be reported")
	})

	t.Run("reports a binding whose rule was deleted behind the framework", func(t *testing.T) {
		binding := seedReconcilableBinding(t)

		// Delete the stored rule directly, the way a stale replica or a manual
		// repair would, leaving the record behind.
		rules := make([]*modelauthz.CasbinRule, 0)
		require.NoError(t, database.Database[*modelauthz.CasbinRule](ctx).
			WithQuery(&modelauthz.CasbinRule{
				Ptype: "g", V0: binding.SubjectID, V1: binding.RoleID, V2: tenant.Default,
			}).List(&rules))
		require.Len(t, rules, 1, "the binding should have stored exactly one grouping rule")
		require.NoError(t, database.Database[*modelauthz.CasbinRule](ctx).Delete(rules...))

		report, err := authz.ReconcilePolicies(ctx)
		require.NoError(t, err)
		require.Contains(t, report.Drifts, authz.PolicyDrift{
			Kind: "binding", Direction: "missing",
			Tenant: tenant.Default, Role: binding.RoleID, Subject: binding.SubjectID,
		}, "a binding record with no stored rule should be reported")
	})
}

// TestReconcilePoliciesSkipsRulesNoRecordDerives guards the two rule kinds the
// comparison must leave alone. Judging them against the records would report
// the framework's own baseline as drift on every run.
func TestReconcilePoliciesSkipsRulesNoRecordDerives(t *testing.T) {
	ctx := context.Background()

	require.NoError(t, rbac.RBAC().SetPermissionsForAuthenticated(ctx, []types.Permission{
		{Object: "/api/reconcile/open", Action: "GET"},
	}))
	t.Cleanup(func() { _ = rbac.RBAC().SetPermissionsForAuthenticated(ctx, nil) })

	report, err := authz.ReconcilePolicies(ctx)
	require.NoError(t, err)
	require.True(t, report.InSync(), "unexpected drift: %+v", report.Drifts)

	// The authenticated policy above and the root system role seeded at startup
	// are both counted rather than judged.
	require.GreaterOrEqual(t, report.Skipped, 2)

	for _, drift := range report.Drifts {
		require.NotEqual(t, consts.AUTHZ_ROLE_AUTHENTICATED, drift.Role)
	}
}

// seedReconcilableBinding creates a role and a binding to it through the normal
// path, so both the records and their stored rules exist before a test perturbs
// one of them.
func seedReconcilableBinding(t *testing.T) *modelauthz.RoleBinding {
	t.Helper()
	ctx := context.Background()

	role := &modelauthz.Role{Name: "reconcile-" + util.UUID()}
	require.NoError(t, database.Database[*modelauthz.Role](ctx).Create(role))

	binding := &modelauthz.RoleBinding{SubjectID: util.UUID(), RoleID: role.ID}
	require.NoError(t, database.Database[*modelauthz.RoleBinding](ctx).Create(binding))

	t.Cleanup(func() {
		_ = database.Database[*modelauthz.RoleBinding](ctx).Delete(binding)
		_ = database.Database[*modelauthz.Role](ctx).Delete(role)
	})
	return binding
}
