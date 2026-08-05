package rbac

import (
	"context"
	"slices"
	"testing"

	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/hydroan/gst/types/consts"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestAuthorizeNamesTheGrantingRule covers one case per branch a decision can
// take, plus the denials that have no granting rule to name and report what
// they were missing instead.
func TestAuthorizeNamesTheGrantingRule(t *testing.T) {
	r := newAuthorizeFixture(t)
	ctx := context.Background()

	cases := []struct {
		name        string
		subject     string
		object      string
		wantAllowed bool
		wantSource  consts.GrantSource
		wantReason  consts.DenyReason
		wantRule    []string
	}{
		{
			name:        "system role",
			subject:     "u_system",
			object:      "/api/things",
			wantAllowed: true,
			wantSource:  consts.GrantSourceSystemRoot,
		},
		{
			name:        "tenant admin",
			subject:     "u_admin",
			object:      "/api/things",
			wantAllowed: true,
			wantSource:  consts.GrantSourceTenantAdmin,
		},
		{
			name:        "authenticated policy",
			subject:     "u_plain",
			object:      "/api/open",
			wantAllowed: true,
			wantSource:  consts.GrantSourceAuthenticated,
			wantRule:    []string{"*", consts.AUTHZ_ROLE_AUTHENTICATED, "/api/open", "GET", "allow"},
		},
		{
			name:        "role policy",
			subject:     "u_member",
			object:      "/api/things",
			wantAllowed: true,
			wantSource:  consts.GrantSourceRole,
			wantRule:    []string{"default", "role_a", "/api/things", "GET", "allow"},
		},
		{
			name:        "denied for holding no role",
			subject:     "u_plain",
			object:      "/api/things",
			wantAllowed: false,
			wantReason:  consts.DenyReasonNoRole,
		},
		{
			name:        "denied for holding no permission",
			subject:     "u_member",
			object:      "/api/unreachable",
			wantAllowed: false,
			wantReason:  consts.DenyReasonNoPolicy,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decision, err := r.Authorize(ctx, "default", c.subject, c.object, "GET")
			allowed := decision.Allowed
			source := decision.Source
			rule := decision.MatchedRule
			if err != nil {
				t.Fatal(err)
			}
			if allowed != c.wantAllowed {
				t.Fatalf("allowed: expected %v, got %v", c.wantAllowed, allowed)
			}
			if source != c.wantSource {
				t.Errorf("source: expected %q, got %q", c.wantSource, source)
			}
			if decision.Reason != c.wantReason {
				t.Errorf("reason: expected %q, got %q", c.wantReason, decision.Reason)
			}
			if !slices.Equal(rule, c.wantRule) {
				t.Errorf("rule: expected %v, got %v", c.wantRule, rule)
			}
		})
	}
}

// TestAuthorizeReportsStrongestSource pins the branch order. A subject
// holding both a system role and a role policy for the same object is allowed by
// either one, and only the stronger answers the question the field exists for:
// revoking the role policy would not take the access away.
func TestAuthorizeReportsStrongestSource(t *testing.T) {
	r := newAuthorizeFixture(t)
	seed(t, tenantRoleGrouping, []string{"u_system", "role_a", "default"})

	decision, err := r.Authorize(context.Background(), "default", "u_system", "/api/things", "GET")
	source := decision.Source
	rule := decision.MatchedRule
	if err != nil {
		t.Fatal(err)
	}
	if source != consts.GrantSourceSystemRoot {
		t.Errorf("expected the system role to outrank the role policy, got %q", source)
	}
	if rule != nil {
		t.Errorf("expected no rule for a source that consults none, got %v", rule)
	}
}

// TestAuthorizeOmitsRuleForPolicylessBranches guards the reason the
// rule is withheld above: those two branches are answered before the engine is
// entered, so no stored row took part in the decision and naming one would
// point at a grant that is not the reason for the access — here, for an
// entirely different object.
func TestAuthorizeOmitsRuleForPolicylessBranches(t *testing.T) {
	r := newAuthorizeFixture(t)
	ctx := context.Background()

	for _, subject := range []string{"u_system", "u_admin"} {
		decision, err := r.Authorize(ctx, "default", subject, "/api/unrelated", "DELETE")
		rule := decision.MatchedRule
		if err != nil {
			t.Fatal(err)
		}
		if rule != nil {
			t.Errorf("%s: expected no rule, got %v", subject, rule)
		}
	}
}

// TestAuthorizeResolvesInheritedRoleLinks pins what deciding the two role
// branches outside the matcher must not change.
//
// The matcher asked Casbin's g function, which resolves a subject reaching a
// role through another role. A lookup of the grouping rules as written answers
// no to both cases below, so a branch rebuilt that way would take away access
// that used to be granted, and would do it silently.
func TestAuthorizeResolvesInheritedRoleLinks(t *testing.T) {
	r := newAuthorizeFixture(t)
	ctx := context.Background()

	for _, grouping := range [][]string{
		{"u_relayed_system", "relay_role"},
		{"relay_role", consts.AUTHZ_SYSTEM_ROLE_ROOT},
	} {
		seed(t, systemRoleGrouping, grouping)
	}
	for _, grouping := range [][]string{
		{"u_relayed_admin", "relay_tenant_role", "default"},
		{"relay_tenant_role", consts.AUTHZ_ROLE_ADMIN, "default"},
	} {
		seed(t, tenantRoleGrouping, grouping)
	}

	cases := []struct {
		name       string
		subject    string
		wantSource consts.GrantSource
	}{
		{"system role through another role", "u_relayed_system", consts.GrantSourceSystemRoot},
		{"tenant admin through another role", "u_relayed_admin", consts.GrantSourceTenantAdmin},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decision, err := r.Authorize(ctx, "default", c.subject, "/api/unrelated", "DELETE")
			allowed := decision.Allowed
			source := decision.Source
			if err != nil {
				t.Fatal(err)
			}
			if !allowed {
				t.Fatalf("expected %s to reach the role through relay_role", c.subject)
			}
			if source != c.wantSource {
				t.Errorf("source: expected %q, got %q", c.wantSource, source)
			}
		})
	}
}

// TestHasSystemRoleAgreesWithAuthorize pins the two entry points that answer
// whether a subject holds a system role to one resolution.
//
// Authorize grants a system_root subject every object in every tenant, and
// every caller of HasSystemRole is a guard over that grant: refusing a tenant
// administrator a root target, exempting root from menu filtering, reporting
// root at login. An answer weaker than Authorize's does not merely disagree,
// it leaves those guards open for the one subject the grant is widest for.
func TestHasSystemRoleAgreesWithAuthorize(t *testing.T) {
	r := newAuthorizeFixture(t)
	ctx := context.Background()

	for _, grouping := range [][]string{
		{"u_relayed_system", "relay_role"},
		{"relay_role", consts.AUTHZ_SYSTEM_ROLE_ROOT},
	} {
		seed(t, systemRoleGrouping, grouping)
	}

	cases := []struct {
		name    string
		subject string
		want    bool
	}{
		{"assigned directly", "u_system", true},
		{"reached through another role", "u_relayed_system", true},
		{"holds no system role", "u_member", false},
		{"named after the role", consts.AUTHZ_SYSTEM_ROLE_ROOT, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			held, err := r.HasSystemRole(ctx, c.subject, consts.AUTHZ_SYSTEM_ROLE_ROOT)
			if err != nil {
				t.Fatal(err)
			}
			if held != c.want {
				t.Errorf("HasSystemRole: expected %v, got %v", c.want, held)
			}

			decision, err := r.Authorize(ctx, "default", c.subject, "/api/unrelated", "DELETE")
			if err != nil {
				t.Fatal(err)
			}
			if grantedAsRoot := decision.Source == consts.GrantSourceSystemRoot; grantedAsRoot != held {
				t.Errorf(
					"Authorize reported source %q while HasSystemRole reported %v",
					decision.Source, held,
				)
			}
		})
	}
}

// TestAuthorizeRejectsSubjectsNamedAfterARole covers Casbin's self-match: its
// role manager answers yes whenever the subject and the role are the same name,
// which would hand tenant-wide or cross-tenant access to whoever registers
// under that name. The matcher guarded against it with an inequality, and the
// branches still have to.
func TestAuthorizeRejectsSubjectsNamedAfterARole(t *testing.T) {
	r := newAuthorizeFixture(t)
	ctx := context.Background()

	for _, subject := range []string{consts.AUTHZ_SYSTEM_ROLE_ROOT, consts.AUTHZ_ROLE_ADMIN} {
		t.Run(subject, func(t *testing.T) {
			decision, err := r.Authorize(ctx, "default", subject, "/api/unrelated", "DELETE")
			allowed := decision.Allowed
			source := decision.Source
			if err != nil {
				t.Fatal(err)
			}
			if allowed {
				t.Errorf("a subject named %q must not receive that role, got source %q", subject, source)
			}
		})
	}
}

// TestAuthorizeWithoutPoliciesAnswersTheRoleBranches covers a deployment whose
// policy table is empty, which is what a fresh install is before any role is
// configured. Casbin evaluates its matcher differently when there is no policy
// to iterate, so the branches decided outside it are the only thing keeping the
// built-in roles reachable at that point.
func TestAuthorizeWithoutPoliciesAnswersTheRoleBranches(t *testing.T) {
	r := newTestRBAC(t, 0)
	ctx := context.Background()

	seed(t, systemRoleGrouping, []string{"u_system", consts.AUTHZ_SYSTEM_ROLE_ROOT})
	seed(t, tenantRoleGrouping, []string{"u_admin", consts.AUTHZ_ROLE_ADMIN, "default"})

	cases := []struct {
		name        string
		subject     string
		wantAllowed bool
		wantSource  consts.GrantSource
	}{
		{"system role", "u_system", true, consts.GrantSourceSystemRoot},
		{"tenant admin", "u_admin", true, consts.GrantSourceTenantAdmin},
		{"subject holding neither", "u_plain", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decision, err := r.Authorize(ctx, "default", c.subject, "/api/things", "GET")
			allowed := decision.Allowed
			source := decision.Source
			if err != nil {
				t.Fatal(err)
			}
			if allowed != c.wantAllowed {
				t.Fatalf("allowed: expected %v, got %v", c.wantAllowed, allowed)
			}
			if source != c.wantSource {
				t.Errorf("source: expected %q, got %q", c.wantSource, source)
			}
		})
	}
}

// TestAuthorizeReadsStoredObjectsAsTemplates covers the two ways a stored
// object used to reach past the route it names.
//
// An object is data: it comes from a menu route binding, or from a permission
// an application writes. Read as a regular expression, a metacharacter in one
// reaches routes the policy never named, and one that does not compile fails
// every request that reaches it. Both are exercised by a single denial, which
// evaluates the whole policy set on its way to answering no.
func TestAuthorizeReadsStoredObjectsAsTemplates(t *testing.T) {
	r := newAuthorizeFixture(t)
	ctx := context.Background()

	// Both rows are grants held by u_member, who holds role_a.
	for _, policy := range [][]string{
		{"default", "role_a", "/api/.*", "GET", "allow"},
		{"default", "role_a", "/api/items/[", "GET", "allow"},
	} {
		seed(t, "p", policy)
	}

	decision, err := r.Authorize(ctx, "default", "u_member", "/api/authz/roles", "GET")
	allowed := decision.Allowed
	if err != nil {
		t.Fatalf("a stored template that cannot compile must not fail unrelated decisions: %v", err)
	}
	if allowed {
		t.Error(`a stored "/api/.*" must not reach a route it does not name`)
	}

	// The grants the two rows do carry are the paths they spell, and the
	// ordinary ones around them are untouched.
	for _, object := range []string{"/api/.*", "/api/items/[", "/api/things"} {
		decision, err = r.Authorize(ctx, "default", "u_member", object, "GET")
		allowed := decision.Allowed
		if err != nil {
			t.Fatal(err)
		}
		if !allowed {
			t.Errorf("expected %q to stay granted", object)
		}
	}
}

func newAuthorizeFixture(t *testing.T) *rbac {
	t.Helper()

	r := newTestRBAC(t, 0)
	for _, policy := range [][]string{
		{"default", "role_a", "/api/things", "GET", "allow"},
		{"*", consts.AUTHZ_ROLE_AUTHENTICATED, "/api/open", "GET", "allow"},
	} {
		seed(t, "p", policy)
	}
	for _, grouping := range [][]string{
		{"u_member", "role_a", "default"},
		{"u_admin", consts.AUTHZ_ROLE_ADMIN, "default"},
	} {
		seed(t, tenantRoleGrouping, grouping)
	}
	seed(t, systemRoleGrouping, []string{"u_system", consts.AUTHZ_SYSTEM_ROLE_ROOT})
	return r
}

// TestAuthorizeCountsEveryDecision covers the counter an operator reads to tell
// a deployment denying nothing from one denying everything.
//
// The three outcomes are kept apart because they answer different questions: an
// error is not a denial, and folding it into one would move the count the other
// is read for. Each carries only the label its effect can fill, so a denial
// counted by reason cannot be mistaken for one allowed by a rule kind.
func TestAuthorizeCountsEveryDecision(t *testing.T) {
	prommetrics.AuthzDecisionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "authz_decisions_probe"},
		[]string{"effect", "allowed_by", "denied_by"},
	)
	t.Cleanup(func() { prommetrics.AuthzDecisionsTotal = nil })

	r := newAuthorizeFixture(t)
	ctx := context.Background()

	// One of each outcome: a member reaching its role's object, a subject
	// holding no role at all, and that same member reaching an object no
	// policy covers. The last two are both denials and are counted apart,
	// because a missing binding and a missing permission are repaired
	// differently.
	for _, c := range []struct{ subject, object string }{
		{"u_member", "/api/things"},
		{"u_plain", "/api/things"},
		{"u_member", "/api/unreachable"},
	} {
		if _, err := r.Authorize(ctx, "default", c.subject, c.object, "GET"); err != nil {
			t.Fatal(err)
		}
	}

	for _, c := range []struct {
		effect    string
		allowedBy string
		deniedBy  string
		want      float64
	}{
		{string(consts.EffectAllow), string(consts.GrantSourceRole), "", 1},
		{string(consts.EffectDeny), "", string(consts.DenyReasonNoRole), 1},
		{string(consts.EffectDeny), "", string(consts.DenyReasonNoPolicy), 1},
	} {
		var metric dto.Metric
		counter, err := prommetrics.AuthzDecisionsTotal.GetMetricWithLabelValues(c.effect, c.allowedBy, c.deniedBy)
		if err != nil {
			t.Fatal(err)
		}
		if err := counter.Write(&metric); err != nil {
			t.Fatal(err)
		}
		if got := metric.GetCounter().GetValue(); got != c.want {
			t.Errorf("%s/%s/%s: expected %v, got %v", c.effect, c.allowedBy, c.deniedBy, c.want, got)
		}
	}
}
