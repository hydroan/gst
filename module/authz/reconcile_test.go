package authz_test

import (
	"context"
	"testing"
	"uuid"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/module/authz"
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
		role := uuid.NewV7().String()
		object := "/api/reconcile/" + role
		require.NoError(t, rbac.RBAC().SetRolePermissions(ctx, tenant.Default, role, []types.Permission{
			{Object: object, Action: "GET"},
		}))
		t.Cleanup(func() { _ = rbac.RBAC().SetRolePermissions(ctx, tenant.Default, role, nil) })

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
		rules := make([]*modelauthz.AuthzRule, 0)
		require.NoError(t, database.Database[*modelauthz.AuthzRule](ctx).
			WithQuery(&modelauthz.AuthzRule{
				Ptype: "g", V0: binding.SubjectID, V1: binding.RoleID, V2: tenant.Default,
			}).List(&rules))
		require.Len(t, rules, 1, "the binding should have stored exactly one grouping rule")
		require.NoError(t, database.Database[*modelauthz.AuthzRule](ctx).Delete(rules...))

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

	report, err := authz.ReconcilePolicies(ctx)
	require.NoError(t, err)
	require.True(t, report.InSync(), "unexpected drift: %+v", report.Drifts)

	// The authenticated policy above and the root system role seeded at startup
	// are both counted rather than judged.
	require.GreaterOrEqual(t, report.Skipped, 2)

	for _, drift := range report.Drifts {
		require.NotEqual(t, rbac.RoleAuthenticated, drift.Role)
	}
}

// TestReconcilePoliciesJudgesTheBuiltInAdminAssignmentByItsBinding covers the
// assignment that grants the built-in tenant administrator role. The role has
// no row in the roles table, so the binding naming it is the one record the
// assignment is derived from: made through the binding it is in sync, made
// around the framework it is an orphaned binding.
func TestReconcilePoliciesJudgesTheBuiltInAdminAssignmentByItsBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("made through a binding", func(t *testing.T) {
		binding := &modelauthz.RoleBinding{SubjectID: uuid.NewV7().String(), RoleID: rbac.RoleAdmin}
		require.NoError(t, database.Database[*modelauthz.RoleBinding](ctx).Create(binding))
		t.Cleanup(func() { require.NoError(t, database.Database[*modelauthz.RoleBinding](ctx).Delete(binding)) })

		report, err := authz.ReconcilePolicies(ctx)
		require.NoError(t, err)
		require.True(t, report.InSync(), "unexpected drift: %+v", report.Drifts)
	})

	t.Run("made around the framework", func(t *testing.T) {
		subject := uuid.NewV7().String()
		require.NoError(t, rbac.RBAC().AssignRole(ctx, tenant.Default, subject, rbac.RoleAdmin))
		t.Cleanup(func() {
			require.NoError(t, rbac.RBAC().UnassignRole(ctx, tenant.Default, subject, rbac.RoleAdmin))
		})

		report, err := authz.ReconcilePolicies(ctx)
		require.NoError(t, err)
		require.Contains(t, report.Drifts, authz.PolicyDrift{
			Kind: "binding", Direction: "orphaned",
			Tenant: tenant.Default, Role: rbac.RoleAdmin, Subject: subject,
		}, "an assignment with no binding behind it should be reported")
	})
}

// seedReconcilableBinding creates a role and a binding to it through the normal
// path, so both the records and their stored rules exist before a test perturbs
// one of them.
func seedReconcilableBinding(t *testing.T) *modelauthz.RoleBinding {
	t.Helper()
	ctx := context.Background()

	role := &modelauthz.Role{Name: "reconcile-" + uuid.NewV7().String()}
	require.NoError(t, database.Database[*modelauthz.Role](ctx).Create(role))

	binding := &modelauthz.RoleBinding{SubjectID: uuid.NewV7().String(), RoleID: role.ID}
	require.NoError(t, database.Database[*modelauthz.RoleBinding](ctx).Create(binding))

	// The binding must go: this fixture deliberately drops its policy rule, so
	// a surviving record shows up as drift in the InSync assertions that follow.
	// The role itself derives no expected policy, so it can stay.
	t.Cleanup(func() {
		_ = database.Database[*modelauthz.RoleBinding](ctx).Delete(binding)
	})
	return binding
}
