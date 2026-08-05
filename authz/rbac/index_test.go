package rbac

import (
	"context"
	"math/rand"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// matcherFuncPathMatch is the name modelData's matcher calls pathMatch by. It
// lives with the oracle: the runtime stopped evaluating the matcher, so only
// the engine the differential test builds still resolves the name.
const matcherFuncPathMatch = "pathMatch"

// pathMatchFunc adapts pathMatch to the signature the oracle's matcher calls
// it with. The argument checks mirror Casbin's own: the matcher passes tokens
// straight through, so a model naming the wrong token reaches this function
// with a value that is not a string, and reporting that is more use than a
// type assertion panic recovered several frames away.
func pathMatchFunc(args ...any) (any, error) {
	if len(args) != 2 {
		return false, errors.Newf("%s: expected 2 arguments, got %d", matcherFuncPathMatch, len(args))
	}
	path, ok := args[0].(string)
	if !ok {
		return false, errors.Newf("%s: argument 1 must be a string, got %T", matcherFuncPathMatch, args[0])
	}
	template, ok := args[1].(string)
	if !ok {
		return false, errors.Newf("%s: argument 2 must be a string, got %T", matcherFuncPathMatch, args[1])
	}
	return pathMatch(path, template)
}

// modelData is the Casbin model the runtime's semantics are specified in. It
// lives with its only consumer: the oracle below evaluates it with the engine
// over the same rules the runtime answers from its own index and graph, so a
// semantic drift between this text and the implementation is a test failure
// rather than a silent split.
var modelData = []byte(`
[request_definition]
# r defines the incoming authorization request tuple.
# tenant: authorization domain, defaults to "default"
# sub: subject, usually the authenticated user ID
# obj: object, usually the requested API path
# act: action, usually the HTTP method
r = tenant, sub, obj, act

[policy_definition]
# p defines a permission granted to role inside tenant.
# tenant: authorization domain
# role: role identifier stored in authz role bindings
# obj: object template, for example /api/authz/roles/{id}
# act: action, usually the HTTP method
# eft: effect, currently "allow"
p = tenant, role, obj, act, eft

[role_definition]
# g defines role membership inside a tenant:
# g(subject, role, tenant) means subject has role in tenant.
g = _, _, _
# g2 defines system-level role membership:
# g2(subject, role) means subject has role outside any tenant.
# The matcher below never names g2. It is declared so Casbin builds a role
# manager for it, which is what authorize asks about system-level membership;
# removing it would leave that question with nothing to answer it.
g2 = _, _

[policy_effect]
# Allow the request if any matched policy effect is "allow".
e = some(where (p.eft == allow))

[matchers]
# This matcher is the specification of a policy decision, not its execution.
# Decisions answer from the decision index, which partitions the same rules by
# the equalities below; the engine would have evaluated this expression once
# per stored rule on every request, at an interpreter's price per row. The
# differential tests hold the index to this text, so a semantic change here
# without a matching index change is a test failure rather than a silent split.
# The two branches authorize decides before consulting any policy — system_root
# and the built-in admin role inside the request tenant — are deliberately not
# spelled here: neither reads p, so no rule is ever the reason for their access.
#
# Allow a request when either:
# 1) the policy is written for the implicit "authenticated" role. This branch
#    checks no role membership and no tenant, so it reaches every authenticated
#    subject in every tenant — including subjects that hold no role at all. It
#    still requires a subject: authorization runs after authentication, and the
#    emptiness check keeps that a property of the specification rather than a
#    promise the caller has to keep. Reserve it for objects whose result is
#    already scoped to the caller.
# 2) the subject belongs to the policy role in the same tenant, and the object
#    and action match the stored permission.
#
# The subject/role inequality check keeps a subject named like a role from
# receiving that role through Casbin's self-match behavior.
#
# A trailing backslash continues the line, so each branch above maps to the line
# below it and the whole matcher stays readable as it grows. A comment between
# two continued lines would end the value early, so keep the two lines adjacent.
m = (r.sub != "" && p.role == "authenticated" && pathMatch(r.obj, p.obj) && r.act == p.act) \
 || (r.sub != p.role && r.tenant == p.tenant && g(r.sub, p.role, r.tenant) && pathMatch(r.obj, p.obj) && r.act == p.act)
`)

// newOracle builds the engine that evaluates modelData as written — the
// specification the runtime's index and graph are held to.
func newOracle(tb testing.TB) *casbin.Enforcer {
	tb.Helper()
	m, err := casbinmodel.NewModelFromString(string(modelData))
	require.NoError(tb, err)
	oracle, err := casbin.NewEnforcer(m)
	require.NoError(tb, err)
	oracle.AddFunction(matcherFuncPathMatch, pathMatchFunc)
	return oracle
}

// TestDecisionIndexAgreesWithTheMatcher holds the runtime to its
// specification. The matcher in modelData is no longer what executes, but it
// remains what the implementation means, and the engine evaluating it over the
// same rules is the one oracle that can say so: every round seeds a randomized
// set of rules and assignments — inheritance chains, a subject named like a
// role, rules whose effect is not allow, authenticated rules beside role rules
// — into both the oracle and the process's own policy set, and every decision
// the runtime answers has to agree with what the engine enforces.
//
// Decisions are compared, not attributions: which of several equally true
// rules a grant names is answered in precedence order by design, where the
// engine answered in storage order. The rule the runtime names is instead held
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
		// The fixture has to hold exactly what the oracle holds, so it starts
		// empty rather than with newTestRBAC's baseline assignment.
		r := newEmptyRBAC(t)
		oracle := newOracle(t)

		seedBoth := func(ptype string, rule []string) {
			var err error
			switch ptype {
			case "p":
				_, err = oracle.AddPolicy(rule)
			case tenantRoleGrouping:
				_, err = oracle.AddGroupingPolicy(rule)
			default:
				_, err = oracle.AddNamedGroupingPolicy(ptype, rule)
			}
			require.NoError(t, err)
			seed(t, ptype, rule)
		}

		for range 30 {
			tenant := tenants[rng.Intn(len(tenants))]
			role := roles[rng.Intn(len(roles))]
			if rng.Intn(6) == 0 {
				tenant, role = authenticatedPolicyTenant, consts.AUTHZ_ROLE_AUTHENTICATED
			}
			seedBoth("p", []string{
				tenant, role,
				templates[rng.Intn(len(templates))],
				actions[rng.Intn(len(actions))],
				effects[rng.Intn(len(effects))],
			})
		}
		// Assignments link subjects to roles and, often enough to matter,
		// roles to roles, which is what exercises the closure.
		members := []string{"u1", "u2", "u3", "role_a", "role_b", "role_c"}
		for range 12 {
			seedBoth(tenantRoleGrouping, []string{
				members[rng.Intn(len(members))],
				roles[rng.Intn(len(roles))],
				tenants[rng.Intn(len(tenants))],
			})
		}

		for _, tenant := range tenants {
			for _, subject := range subjects {
				for _, object := range objects {
					for _, action := range actions {
						want, err := oracle.Enforce(tenant, subject, object, action)
						require.NoError(t, err)

						decision, err := r.Authorize(ctx, tenant, subject, object, action)
						require.NoError(t, err)
						require.Equal(t, want, decision.Allowed,
							"round %d: engine and runtime disagree on (%s, %s, %s, %s): source=%q rule=%v rules=%v",
							round, tenant, subject, object, action,
							decision.Source, decision.MatchedRule, memoryRules(t, r))

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
// first; the runtime answers in the order of the branches — authenticated
// ahead of the roles, because no role revocation can take away what the
// implicit role allows — and, among a subject's roles, in sorted order, so the
// same decision names the same rule on every replay.
func TestAuthorizeAttributesInPrecedenceOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("authenticated outranks a role", func(t *testing.T) {
		r := newTestRBAC(t, 0)
		// The role rule is seeded first, so storage order would name it.
		seed(t, "p", []string{"default", "role_a", "/api/both", "GET", "allow"})
		seed(t, "p", []string{authenticatedPolicyTenant, consts.AUTHZ_ROLE_AUTHENTICATED, "/api/both", "GET", "allow"})
		seed(t, tenantRoleGrouping, []string{"u1", "role_a", "default"})

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
		// role_b's rule is seeded first, so storage order would name it.
		seed(t, "p", []string{"default", "role_b", "/api/shared", "GET", "allow"})
		seed(t, "p", []string{"default", "role_a", "/api/shared", "GET", "allow"})
		for _, role := range []string{"role_b", "role_a"} {
			seed(t, tenantRoleGrouping, []string{"u1", role, "default"})
		}

		decision, err := r.Authorize(ctx, "default", "u1", "/api/shared", "GET")
		require.NoError(t, err)
		require.True(t, decision.Allowed)
		require.Equal(t, consts.GrantSourceRole, decision.Source)
		require.Equal(t,
			[]string{"default", "role_a", "/api/shared", "GET", "allow"},
			decision.MatchedRule)
	})
}
