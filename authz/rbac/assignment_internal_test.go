package rbac

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTenantsForSubjectReadsEveryTenantTheSubjectHoldsARoleIn covers the
// third reading of the assignment relation: the tenants come back once each
// and sorted whatever order the assignments were made in, the built-in
// administrator role counts like any other, a system-level role names no
// tenant, and an unassigned role takes its tenant away.
func TestTenantsForSubjectReadsEveryTenantTheSubjectHoldsARoleIn(t *testing.T) {
	r := newEmptyRBAC(t)
	ctx := context.Background()
	require.NoError(t, r.AssignRole(ctx, "beta", "u1", "viewer"))
	require.NoError(t, r.AssignRole(ctx, "alpha", "u1", "editor"))
	require.NoError(t, r.AssignRole(ctx, "alpha", "u1", RoleAdmin))
	require.NoError(t, r.AssignRole(ctx, "gamma", "u2", "viewer"))
	require.NoError(t, r.AssignSystemRole(ctx, "u1", SystemRoleRoot))

	tenants, err := r.TenantsForSubject(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, tenants, "every tenant a role is held in, once, sorted, and no system-level one")

	tenants, err = r.TenantsForSubject(ctx, " ")
	require.NoError(t, err)
	assert.Empty(t, tenants, "a blank subject holds nothing")

	require.NoError(t, r.UnassignRole(ctx, "beta", "u1", "viewer"))
	tenants, err = r.TenantsForSubject(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha"}, tenants, "a tenant goes with the last role held in it")
}
