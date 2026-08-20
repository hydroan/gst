package authz_test

import (
	"context"
	"testing"
	"uuid"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/database"
	modelauthz "github.com/hydroan/gst/internal/model/authz"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

// TestPolicyWritesRollBackWithTheTransaction is the regression this whole
// mechanism exists for.
//
// Policy changes used to be written on the adapter's own connection, so they
// committed even when the transaction around the model hook that made them
// rolled back. That left an authorization behind with no record justifying it:
// no role row, no binding row, and nothing in the admin interface to revoke.
func TestPolicyWritesRollBackWithTheTransaction(t *testing.T) {
	ctx := context.Background()
	errRollback := errors.New("rollback the transaction")

	t.Run("granting a permission", func(t *testing.T) {
		tenant, role := tenant.Default, uuid.NewV7().String()
		object := "/api/rollback/" + role

		err := database.Transaction(ctx, func(ctx context.Context) error {
			if err := rbac.RBAC().SetRolePermissions(ctx, tenant, role, []types.Permission{{Object: object, Action: "GET"}}); err != nil {
				return err
			}
			return errRollback
		})
		require.ErrorIs(t, err, errRollback)

		requireNoStoredPolicy(t, "p", tenant, role)
		decision, err := rbac.RBAC().Authorize(ctx, tenant, "subject-"+role, object, "GET")
		allowed := decision.Allowed
		require.NoError(t, err)
		require.False(t, allowed, "a rolled back grant must not stay in force in memory")
	})

	t.Run("assigning a role", func(t *testing.T) {
		tenant, role := tenant.Default, uuid.NewV7().String()
		subject := uuid.NewV7().String()

		err := database.Transaction(ctx, func(ctx context.Context) error {
			if err := rbac.RBAC().AssignRole(ctx, tenant, subject, role); err != nil {
				return err
			}
			return errRollback
		})
		require.ErrorIs(t, err, errRollback)

		requireNoStoredPolicy(t, "g", subject, role)
		roles, err := rbac.RBAC().RolesForSubject(ctx, tenant, subject)
		require.NoError(t, err)
		require.NotContains(t, roles, role, "a rolled back assignment must not stay in force in memory")
	})

	t.Run("replacing a role's permissions", func(t *testing.T) {
		tenant, role := tenant.Default, uuid.NewV7().String()
		object := "/api/rollback/" + role

		// A committed set to roll back onto, so the test also shows the
		// previous state survives rather than being cleared by the attempt.
		require.NoError(t, rbac.RBAC().SetRolePermissions(ctx, tenant, role, []types.Permission{
			{Object: object, Action: "GET"},
		}))

		err := database.Transaction(ctx, func(ctx context.Context) error {
			if err := rbac.RBAC().SetRolePermissions(ctx, tenant, role, []types.Permission{
				{Object: object, Action: "DELETE"},
			}); err != nil {
				return err
			}
			return errRollback
		})
		require.ErrorIs(t, err, errRollback)

		rules := storedPolicies(t, "p", tenant, role)
		require.Len(t, rules, 1, "the committed permission should survive the rolled back replacement")
		require.Equal(t, "GET", rules[0].V3)
	})
}

// TestPolicyWritesCommitWithTheTransaction is the other half of the contract:
// deferring the in-memory half until after the commit must not lose it.
func TestPolicyWritesCommitWithTheTransaction(t *testing.T) {
	ctx := context.Background()
	tenant, role := tenant.Default, uuid.NewV7().String()
	subject := uuid.NewV7().String()
	object := "/api/commit/" + role

	require.NoError(t, database.Transaction(ctx, func(ctx context.Context) error {
		if err := rbac.RBAC().SetRolePermissions(ctx, tenant, role, []types.Permission{{Object: object, Action: "GET"}}); err != nil {
			return err
		}
		return rbac.RBAC().AssignRole(ctx, tenant, subject, role)
	}))

	require.Len(t, storedPolicies(t, "p", tenant, role), 1, "the grant should be stored")

	// The grouping rule has to reach the role manager too, not just the policy
	// list, or the subject would hold the role without inheriting its rules.
	decision, err := rbac.RBAC().Authorize(ctx, tenant, subject, object, "GET")
	allowed := decision.Allowed
	require.NoError(t, err)
	require.True(t, allowed, "a committed grant and assignment should authorize the subject")
}

func storedPolicies(t *testing.T, ptype string, v0 string, v1 string) []*modelauthz.AuthzRule {
	t.Helper()

	rules := make([]*modelauthz.AuthzRule, 0)
	require.NoError(t, database.Database[*modelauthz.AuthzRule](context.Background()).
		WithQuery(&modelauthz.AuthzRule{Ptype: ptype, V0: v0, V1: v1}).List(&rules))
	return rules
}

func requireNoStoredPolicy(t *testing.T, ptype string, v0 string, v1 string) {
	t.Helper()
	require.Empty(t, storedPolicies(t, ptype, v0, v1), "a rolled back write must leave no stored rule")
}
