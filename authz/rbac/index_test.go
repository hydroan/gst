package rbac

import (
	"context"
	"math/rand"
	"testing"

	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// TestDecisionIndexAgreesWithTheMatcher holds the index to its specification.
// The matcher in modelData is no longer what executes, but it remains what the
// index means, and the engine evaluating it over the same policy set is the
// one oracle that can say so: every round seeds a randomized set of rules and
// assignments — inheritance chains, a subject named like a role, rules whose
// effect is not allow, authenticated rules beside role rules — and every
// decision the index answers has to agree with what the engine enforces.
//
// Decisions are compared, not attributions: which of several equally true
// rules a grant names is answered in precedence order by design, where the
// engine answered in storage order. The rule the index names is instead held
// to justify the grant on its own: an allow effect, the action compared, the
// template matched.
//
// The seeds are fixed so a failure replays. No round grants the built-in
// admin role or a system role, so the branches ahead of the index never fire
// and the comparison is of the policy branches alone.
func TestDecisionIndexAgreesWithTheMatcher(t *testing.T) {
	tenants := []string{"default", "tenant_a", "tenant_b"}
	roles := []string{"role_a", "role_b", "role_c", "role_d"}
	subjects := []string{"u1", "u2", "u3", "role_a", ""}
	templates := []string{"/api/a", "/api/things/{id}", "/api/files/*", "/api/x/{id}/y", "/api/.*"}
	actions := []string{"GET", "POST"}
	objects := []string{
		"/api/a", "/api/things/42", "/api/things/42/x", "/api/files/a/b",
		"/api/x/1/y", "/api/nope", "/api/.*",
	}
	effects := []string{"allow", "allow", "allow", "deny"}

	ctx := context.Background()
	for round := range 25 {
		rng := rand.New(rand.NewSource(int64(round) + 1))
		r := newTestRBAC(t, 0)

		for range 30 {
			tenant := tenants[rng.Intn(len(tenants))]
			role := roles[rng.Intn(len(roles))]
			if rng.Intn(6) == 0 {
				tenant, role = authenticatedPolicyTenant, consts.AUTHZ_ROLE_AUTHENTICATED
			}
			_, err := r.enforcer.AddPolicy(
				tenant, role,
				templates[rng.Intn(len(templates))],
				actions[rng.Intn(len(actions))],
				effects[rng.Intn(len(effects))],
			)
			require.NoError(t, err)
		}
		// Assignments link subjects to roles and, often enough to matter,
		// roles to roles, which is what exercises the closure.
		members := []string{"u1", "u2", "u3", "role_a", "role_b", "role_c"}
		for range 12 {
			_, err := r.enforcer.AddGroupingPolicy(
				members[rng.Intn(len(members))],
				roles[rng.Intn(len(roles))],
				tenants[rng.Intn(len(tenants))],
			)
			require.NoError(t, err)
		}
		reindex(r)

		for _, tenant := range tenants {
			for _, subject := range subjects {
				for _, object := range objects {
					for _, action := range actions {
						want, err := r.enforcer.Enforce(tenant, subject, object, action)
						require.NoError(t, err)

						decision, err := r.Authorize(ctx, tenant, subject, object, action)
						require.NoError(t, err)
						require.Equal(t, want, decision.Allowed,
							"round %d: engine and index disagree on (%s, %s, %s, %s)",
							round, tenant, subject, object, action)

						if !decision.Allowed {
							continue
						}
						rule := decision.MatchedRule
						require.Len(t, rule, 5, "an allowed policy decision has to name its rule")
						require.Equal(t, string(consts.EffectAllow), rule[4])
						require.Equal(t, action, rule[3])
						matched, err := pathMatch(object, rule[2])
						require.NoError(t, err)
						require.True(t, matched, "the named rule has to justify the grant on its own")
					}
				}
			}
		}
	}
}

// TestAuthorizeAttributesInPrecedenceOrder pins which rule a grant names when
// several are equally true. The engine named whichever the storage order put
// first; the index answers in the order of the branches — authenticated ahead
// of the roles, because no role revocation can take away what the implicit
// role allows — and, among a subject's roles, in sorted order, so the same
// decision names the same rule on every replay.
func TestAuthorizeAttributesInPrecedenceOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("authenticated outranks a role", func(t *testing.T) {
		r := newTestRBAC(t, 0)
		// The role rule is stored first, so storage order would name it.
		_, err := r.enforcer.AddPolicy("default", "role_a", "/api/both", "GET", "allow")
		require.NoError(t, err)
		_, err = r.enforcer.AddPolicy(
			authenticatedPolicyTenant, consts.AUTHZ_ROLE_AUTHENTICATED, "/api/both", "GET", "allow",
		)
		require.NoError(t, err)
		_, err = r.enforcer.AddGroupingPolicy("u1", "role_a", "default")
		require.NoError(t, err)
		reindex(r)

		decision, err := r.Authorize(ctx, "default", "u1", "/api/both", "GET")
		require.NoError(t, err)
		require.True(t, decision.Allowed)
		require.Equal(t, consts.GrantSourceAuthenticated, decision.Source)
		require.Equal(t,
			[]string{authenticatedPolicyTenant, consts.AUTHZ_ROLE_AUTHENTICATED, "/api/both", "GET", "allow"},
			decision.MatchedRule)
	})

	t.Run("roles answer in sorted order", func(t *testing.T) {
		r := newTestRBAC(t, 0)
		// role_b's rule is stored first, so storage order would name it.
		_, err := r.enforcer.AddPolicy("default", "role_b", "/api/shared", "GET", "allow")
		require.NoError(t, err)
		_, err = r.enforcer.AddPolicy("default", "role_a", "/api/shared", "GET", "allow")
		require.NoError(t, err)
		for _, role := range []string{"role_b", "role_a"} {
			_, err = r.enforcer.AddGroupingPolicy("u1", role, "default")
			require.NoError(t, err)
		}
		reindex(r)

		decision, err := r.Authorize(ctx, "default", "u1", "/api/shared", "GET")
		require.NoError(t, err)
		require.True(t, decision.Allowed)
		require.Equal(t, consts.GrantSourceRole, decision.Source)
		require.Equal(t,
			[]string{"default", "role_a", "/api/shared", "GET", "allow"},
			decision.MatchedRule)
	})
}
