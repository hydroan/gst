package authz_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module/authz"
	"github.com/hydroan/gst/tenant"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// The tests in this file are about what a policy set decides, not about what a
// write leaves in the table. Everything around them asserts stored rules —
// which is the wrong half to trust on its own: a decision path that inverted
// its answer, lost the index rebuild, or renamed a grant source would leave
// every one of those assertions green while no request in the deployment was
// authorized correctly. What is asserted here is the request itself getting
// through or being refused, and the decision reporting why.
//
// The route each case grants is one the authz module registers, so a grant is
// observable as an ordinary request rather than through a stub: /api/authz/roles
// GET is used throughout as "the route this role may reach", and
// /api/authz/role-bindings GET as "the route it may not".

const (
	grantedRoute = rolePath
	deniedRoute  = roleBindingPath
)

// authorizationSubject is a signed-up user together with the client that
// presents its session, which is what every case below acts through.
type authorizationSubject struct {
	userID    string
	sessionID string
	client    *client.Client
}

func newAuthorizationSubject(t *testing.T, name string) authorizationSubject {
	t.Helper()
	userID, sessionID := authzSignupAndLoginUser(t, authzTestUsername(name), "12345678")
	return authorizationSubject{userID: userID, sessionID: sessionID, client: authzSessionClient(t, sessionID)}
}

// newAuthorizationRole creates a role bound to the subject and removes it when
// the test ends.
//
// The cleanup is what keeps these cases from being visible to anything else.
// The permissions below are granted directly rather than through a menu, which
// is what makes them expressive here and orphaned everywhere else: the drift
// report derives its expectations from roles and their menus, so a rule with no
// menu behind it reads — correctly — as a rule no record justifies. Deleting
// the role takes its bindings and both kinds of rule with it.
func newAuthorizationRole(t *testing.T, subject authorizationSubject, name string) string {
	t.Helper()

	roleID := authzCreateTenantRole(t, tenant.Default, authzTestUsername(name))
	authzBindTenantRole(t, tenant.Default, subject.userID, roleID)
	t.Cleanup(func() {
		ctx := tenant.In(context.Background(), tenant.Default)
		role := &authz.Role{Base: model.Base{ID: roleID}}
		require.NoError(t, database.Database[*authz.Role](ctx).Delete(role))
	})
	return roleID
}

// requireReaches asserts the subject may list roles, and that the decision
// says so for the reason expected.
func requireReaches(t *testing.T, subject authorizationSubject, route string, source consts.GrantSource) {
	t.Helper()

	_, err := client.Get[client.ListResult[*authz.Role]](subject.client, route)
	require.NoError(t, err, "the request has to be authorized")

	decision, err := rbac.RBAC().Authorize(context.Background(), tenant.Default, subject.userID, route, http.MethodGet)
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.Equal(t, source, decision.Source,
		"the decision has to name what allowed it, or a revocation has nothing to aim at")
}

// requireRefused asserts the subject may not reach the route, and that the
// decision names which of the two steps behind a grant was missing.
func requireRefused(t *testing.T, subject authorizationSubject, route string, reason consts.DenyReason) {
	t.Helper()

	_, err := client.Get[client.ListResult[*authz.RoleBinding]](subject.client, route)
	testutil.RequireError(t, err, http.StatusForbidden, "permission denied")

	decision, err := rbac.RBAC().Authorize(context.Background(), tenant.Default, subject.userID, route, http.MethodGet)
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.Equal(t, reason, decision.Reason,
		"a denial has to name the missing step, or the repair is a guess between a role and a permission")
}

// TestAuthorizationGrantsAndRefusesByRole covers the decision every request in
// a deployment goes through: a role reaches what its permissions name and
// nothing else. The two denials are told apart because they lead to opposite
// repairs — a missing role binding, or a missing permission.
func TestAuthorizationGrantsAndRefusesByRole(t *testing.T) {
	subject := newAuthorizationSubject(t, "authorization_role")

	// Holding no role at all: the denial is about the binding, not the policy.
	requireRefused(t, subject, deniedRoute, consts.DenyReasonNoRole)

	roleID := newAuthorizationRole(t, subject, "authorization_role")
	authzGrantTenantPolicy(t, tenant.Default, roleID, types.Permission{
		Object: grantedRoute, Action: http.MethodGet,
	})

	requireReaches(t, subject, grantedRoute, consts.GrantSourceRole)
	// Holding a role that does not carry this permission.
	requireRefused(t, subject, deniedRoute, consts.DenyReasonNoPolicy)
}

// TestAuthorizationReportsEveryGrantSource covers the three grants that do not
// come from a role's own permissions. Each reaches a request the policy set
// alone would refuse, and each has to be reported as itself: an operator who
// strips a role from a system subject and finds the route still open needs the
// log to have said system_root rather than role.
func TestAuthorizationReportsEveryGrantSource(t *testing.T) {
	t.Run("system_root", func(t *testing.T) {
		// The built-in root subject holds the system role, which reaches every
		// object in every tenant without consulting a policy.
		rootSessionID := authzAdminSessionID(t)
		root := authorizationSubject{
			userID:    consts.AUTHZ_USER_ROOT,
			sessionID: rootSessionID,
			client:    authzSessionClient(t, rootSessionID),
		}
		requireReaches(t, root, grantedRoute, consts.GrantSourceSystemRoot)

		decision, err := rbac.RBAC().Authorize(
			context.Background(), tenant.Default, root.userID, grantedRoute, http.MethodGet,
		)
		require.NoError(t, err)
		require.Empty(t, decision.MatchedRule,
			"a branch that consults no policy must not name a rule as the reason for access")
	})

	t.Run("tenant_admin", func(t *testing.T) {
		// The built-in administrator role is granted by assignment alone: it
		// has no row in the roles table and carries no permissions, so a
		// subject holding it reaches the route on that branch or not at all.
		subject := newAuthorizationSubject(t, "authorization_admin")
		requireRefused(t, subject, deniedRoute, consts.DenyReasonNoRole)

		ctx := context.Background()
		require.NoError(t, rbac.RBAC().AssignRole(ctx, tenant.Default, subject.userID, consts.AUTHZ_ROLE_ADMIN))
		t.Cleanup(func() {
			require.NoError(t, rbac.RBAC().UnassignRole(ctx, tenant.Default, subject.userID, consts.AUTHZ_ROLE_ADMIN))
		})

		requireReaches(t, subject, grantedRoute, consts.GrantSourceTenantAdmin)
	})

	t.Run("authenticated", func(t *testing.T) {
		// Policies written for the implicit role are matched without a role
		// membership check, so they reach a subject holding no role at all.
		subject := newAuthorizationSubject(t, "authorization_authenticated")
		requireRefused(t, subject, deniedRoute, consts.DenyReasonNoRole)

		ctx := context.Background()
		require.NoError(t, rbac.RBAC().SetPermissionsForAuthenticated(ctx, []types.Permission{
			{Object: grantedRoute, Action: http.MethodGet},
		}))
		t.Cleanup(func() {
			require.NoError(t, rbac.RBAC().SetPermissionsForAuthenticated(ctx, nil))
		})

		requireReaches(t, subject, grantedRoute, consts.GrantSourceAuthenticated)
	})
}

// TestAuthorizationFollowsPolicyChangesWithoutReauthentication covers what a
// permission change is expected to mean: it takes effect on the session that
// is already open. The rules reaching storage is not the property that matters
// — the decision path has to pick them up, which is a rebuild away from the
// write.
func TestAuthorizationFollowsPolicyChangesWithoutReauthentication(t *testing.T) {
	subject := newAuthorizationSubject(t, "authorization_live")
	roleID := newAuthorizationRole(t, subject, "authorization_live")
	authzGrantTenantPolicy(t, tenant.Default, roleID, types.Permission{
		Object: grantedRoute, Action: http.MethodGet,
	})
	requireReaches(t, subject, grantedRoute, consts.GrantSourceRole)

	t.Run("a replaced permission set moves the grant", func(t *testing.T) {
		// The same session, no new login: the permission set is replaced, and
		// what the subject may reach follows it in both directions.
		authzGrantTenantPolicy(t, tenant.Default, roleID, types.Permission{
			Object: deniedRoute, Action: http.MethodGet,
		})

		_, err := client.Get[client.ListResult[*authz.Role]](subject.client, grantedRoute)
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")
		_, err = client.Get[client.ListResult[*authz.RoleBinding]](subject.client, deniedRoute)
		require.NoError(t, err, "the newly granted route has to open on the session already in flight")
	})

	t.Run("an unassigned role takes its grants with it", func(t *testing.T) {
		require.NoError(t, rbac.RBAC().UnassignRole(context.Background(), tenant.Default, subject.userID, roleID))

		requireRefused(t, subject, deniedRoute, consts.DenyReasonNoRole)
	})
}

// TestAuthorizationConvergesOnAPolicySetChangedOutsideTheProcess covers the
// staleness a process cannot see for itself: another replica's write, a manual
// repair, a restore. Nothing local reports it — no error, no divergence, no row
// count to compare — so only a reload brings the process back in step. The
// production path is a schedule; the test drives the same entry point directly
// rather than waiting an interval out.
func TestAuthorizationConvergesOnAPolicySetChangedOutsideTheProcess(t *testing.T) {
	subject := newAuthorizationSubject(t, "authorization_external")
	roleID := newAuthorizationRole(t, subject, "authorization_external")
	requireRefused(t, subject, deniedRoute, consts.DenyReasonNoPolicy)

	// Written straight into the table, which is what a write this process did
	// not make looks like: no mutation, no index rebuild, nothing in memory.
	ctx := context.Background()
	granted := &authz.AuthzRule{
		Ptype: "p", V0: tenant.Default, V1: roleID,
		V2: deniedRoute, V3: http.MethodGet, V4: string(consts.EffectAllow),
	}
	require.NoError(t, database.Database[*authz.AuthzRule](ctx).Create(granted))
	t.Cleanup(func() {
		require.NoError(t, database.Database[*authz.AuthzRule](ctx).Delete(granted))
	})

	_, err := client.Get[client.ListResult[*authz.RoleBinding]](subject.client, deniedRoute)
	testutil.RequireError(t, err, http.StatusForbidden, "permission denied")

	require.NoError(t, rbac.RBAC().ReloadPolicies(ctx))

	_, err = client.Get[client.ListResult[*authz.RoleBinding]](subject.client, deniedRoute)
	require.NoError(t, err, "a reload has to pick up a rule this process never wrote")
}

// TestAuthorizationAttributesInPrecedenceOrder covers which grant a decision
// names when several are true at once. The order is what makes the answer
// actionable: revoking the one that was named has to be what takes the access
// away, so the strongest grant is reported and the weaker ones are not.
func TestAuthorizationAttributesInPrecedenceOrder(t *testing.T) {
	subject := newAuthorizationSubject(t, "authorization_precedence")
	roleID := newAuthorizationRole(t, subject, "authorization_precedence")
	authzGrantTenantPolicy(t, tenant.Default, roleID, types.Permission{
		Object: grantedRoute, Action: http.MethodGet,
	})
	requireReaches(t, subject, grantedRoute, consts.GrantSourceRole)

	ctx := context.Background()
	require.NoError(t, rbac.RBAC().SetPermissionsForAuthenticated(ctx, []types.Permission{
		{Object: grantedRoute, Action: http.MethodGet},
	}))
	t.Cleanup(func() {
		require.NoError(t, rbac.RBAC().SetPermissionsForAuthenticated(ctx, nil))
	})

	// Both grants now cover the route. Naming the role would send an operator
	// to revoke it and find the route still open, because the implicit role
	// reaches every subject that can log in.
	requireReaches(t, subject, grantedRoute, consts.GrantSourceAuthenticated)
}
