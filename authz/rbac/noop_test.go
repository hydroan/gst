package rbac

import (
	"context"
	"testing"

	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRBACWithoutAnEnforcerRefusesWrites covers a process holding no policy set:
// RBAC disabled, or one that never bootstrapped.
//
// A write has nowhere to go there — no in-memory set to change, no adapter to
// store the change through — and reporting success would tell the caller its
// change is in force. Nothing downstream can tell that apart from a change that
// landed, because the records a rule is derived from are written either way: a
// role created through the API answers success with not one policy row behind it.
func TestRBACWithoutAnEnforcerRefusesWrites(t *testing.T) {
	r := rbacWithoutEnforcer(t)
	ctx := context.Background()
	permissions := []types.Permission{{Object: "/api/things", Action: "GET"}}

	writes := map[string]func() error{
		"RemoveRole": func() error { return r.RemoveRole(ctx, "tenant_a", "role_a") },
		"SetRolePermissions": func() error {
			return r.SetRolePermissions(ctx, "tenant_a", "role_a", permissions)
		},
		"SetPermissionsForAuthenticated": func() error {
			return r.SetPermissionsForAuthenticated(ctx, permissions)
		},
		"AssignRole":         func() error { return r.AssignRole(ctx, "tenant_a", "u1", "role_a") },
		"UnassignRole":       func() error { return r.UnassignRole(ctx, "tenant_a", "u1", "role_a") },
		"AssignSystemRole":   func() error { return r.AssignSystemRole(ctx, "u1", consts.AUTHZ_SYSTEM_ROLE_ROOT) },
		"UnassignSystemRole": func() error { return r.UnassignSystemRole(ctx, "u1", consts.AUTHZ_SYSTEM_ROLE_ROOT) },
		"RemoveSubject":      func() error { return r.RemoveSubject(ctx, "u1") },
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, write(), ErrRBACDisabled)
		})
	}
}

// TestRBACWithoutAnEnforcerAnswersReads covers the other half of the same
// process. A denial and an empty role set are both true of a deployment that
// stores no policies, so answering them states the situation rather than hiding
// it, and only the built-in root subject is answered otherwise.
func TestRBACWithoutAnEnforcerAnswersReads(t *testing.T) {
	r := rbacWithoutEnforcer(t)
	ctx := context.Background()

	decision, err := r.Authorize(ctx, "tenant_a", consts.AUTHZ_USER_ROOT, "/api/things", "GET")
	require.NoError(t, err)
	assert.False(t, decision.Allowed, "a process deciding from no policies has to deny")
	assert.Equal(t, consts.DenyReasonNotInitialized, decision.Reason,
		"every request is refused the same way, which is a deployment answer rather than a missing rule")

	// The built-in root subject keeps system_root so that modules registering no
	// authz can still use root-only administrative flows.
	systemRoot, err := r.HasSystemRole(ctx, consts.AUTHZ_USER_ROOT, consts.AUTHZ_SYSTEM_ROLE_ROOT)
	require.NoError(t, err)
	assert.True(t, systemRoot)

	systemRoot, err = r.HasSystemRole(ctx, "u1", consts.AUTHZ_SYSTEM_ROLE_ROOT)
	require.NoError(t, err)
	assert.False(t, systemRoot)

	roles, err := r.RolesForSubject(ctx, "tenant_a", "u1")
	require.NoError(t, err)
	assert.Empty(t, roles)

	subjects, err := r.SubjectsInTenant(ctx, "tenant_a")
	require.NoError(t, err)
	assert.Empty(t, subjects)

	assert.NoError(t, r.ReloadPolicies(ctx), "there is no in-memory policy set to rebuild")
}

// rbacWithoutEnforcer returns what RBAC answers with before Init has installed
// an enforcer, and guards that the package is in that state at all: the enforcer
// is a package variable, so a test installing one leaves it behind.
func rbacWithoutEnforcer(t *testing.T) types.RBAC {
	t.Helper()

	r := RBAC()
	require.IsType(t, noop{}, r, "the package still holds an enforcer installed elsewhere")
	return r
}
