package rbac

import (
	"context"
	"slices"
	"testing"

	"github.com/hydroan/gst/types/consts"
)

// TestAuthorizeExplainedNamesTheGrantingRule covers one case per matcher branch,
// plus the denial that has no granting rule at all.
func TestAuthorizeExplainedNamesTheGrantingRule(t *testing.T) {
	r := newExplainedFixture(t)
	ctx := context.Background()

	cases := []struct {
		name        string
		subject     string
		object      string
		wantAllowed bool
		wantSource  consts.GrantSource
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
			name:        "denied",
			subject:     "u_plain",
			object:      "/api/things",
			wantAllowed: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			allowed, source, rule, err := r.AuthorizeExplained(ctx, "default", c.subject, c.object, "GET")
			if err != nil {
				t.Fatal(err)
			}
			if allowed != c.wantAllowed {
				t.Fatalf("allowed: expected %v, got %v", c.wantAllowed, allowed)
			}
			if source != c.wantSource {
				t.Errorf("source: expected %q, got %q", c.wantSource, source)
			}
			if !slices.Equal(rule, c.wantRule) {
				t.Errorf("rule: expected %v, got %v", c.wantRule, rule)
			}

			// Both entry points share one decision, so the cheaper one has
			// nothing of its own to drift from.
			decided, err := r.Authorize(ctx, "default", c.subject, c.object, "GET")
			if err != nil {
				t.Fatal(err)
			}
			if decided != allowed {
				t.Errorf("Authorize: expected %v, got %v", allowed, decided)
			}
		})
	}
}

// TestAuthorizeExplainedReportsStrongestSource pins the branch order. A subject
// holding both a system role and a role policy for the same object is allowed by
// either one, and only the stronger answers the question the field exists for:
// revoking the role policy would not take the access away.
func TestAuthorizeExplainedReportsStrongestSource(t *testing.T) {
	r := newExplainedFixture(t)
	if _, err := r.enforcer.AddGroupingPolicy("u_system", "role_a", "default"); err != nil {
		t.Fatal(err)
	}

	_, source, rule, err := r.AuthorizeExplained(context.Background(), "default", "u_system", "/api/things", "GET")
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

// TestAuthorizeExplainedOmitsRuleForPolicylessBranches guards the reason the
// rule is withheld above: those two branches are answered before the engine is
// entered, so no stored row took part in the decision and naming one would
// point at a grant that is not the reason for the access — here, for an
// entirely different object.
func TestAuthorizeExplainedOmitsRuleForPolicylessBranches(t *testing.T) {
	r := newExplainedFixture(t)
	ctx := context.Background()

	for _, subject := range []string{"u_system", "u_admin"} {
		_, _, rule, err := r.AuthorizeExplained(ctx, "default", subject, "/api/unrelated", "DELETE")
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
	r := newExplainedFixture(t)
	ctx := context.Background()

	for _, grouping := range [][]string{
		{"u_relayed_system", "relay_role"},
		{"relay_role", consts.AUTHZ_SYSTEM_ROLE_ROOT},
	} {
		if _, err := r.enforcer.AddNamedGroupingPolicy(systemRoleGrouping, grouping); err != nil {
			t.Fatal(err)
		}
	}
	for _, grouping := range [][]string{
		{"u_relayed_admin", "relay_tenant_role", "default"},
		{"relay_tenant_role", consts.AUTHZ_ROLE_ADMIN, "default"},
	} {
		if _, err := r.enforcer.AddGroupingPolicy(grouping); err != nil {
			t.Fatal(err)
		}
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
			allowed, source, _, err := r.AuthorizeExplained(ctx, "default", c.subject, "/api/unrelated", "DELETE")
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

// TestAuthorizeRejectsSubjectsNamedAfterARole covers Casbin's self-match: its
// role manager answers yes whenever the subject and the role are the same name,
// which would hand tenant-wide or cross-tenant access to whoever registers
// under that name. The matcher guarded against it with an inequality, and the
// branches still have to.
func TestAuthorizeRejectsSubjectsNamedAfterARole(t *testing.T) {
	r := newExplainedFixture(t)
	ctx := context.Background()

	for _, subject := range []string{consts.AUTHZ_SYSTEM_ROLE_ROOT, consts.AUTHZ_ROLE_ADMIN} {
		t.Run(subject, func(t *testing.T) {
			allowed, source, _, err := r.AuthorizeExplained(ctx, "default", subject, "/api/unrelated", "DELETE")
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

	if _, err := r.enforcer.AddNamedGroupingPolicy(
		systemRoleGrouping, "u_system", consts.AUTHZ_SYSTEM_ROLE_ROOT,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := r.enforcer.AddGroupingPolicy("u_admin", consts.AUTHZ_ROLE_ADMIN, "default"); err != nil {
		t.Fatal(err)
	}

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
			allowed, source, _, err := r.AuthorizeExplained(ctx, "default", c.subject, "/api/things", "GET")
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

func newExplainedFixture(t *testing.T) *rbac {
	t.Helper()

	r := newTestRBAC(t, 0)
	for _, policy := range [][]string{
		{"default", "role_a", "/api/things", "GET", "allow"},
		{"*", consts.AUTHZ_ROLE_AUTHENTICATED, "/api/open", "GET", "allow"},
	} {
		if _, err := r.enforcer.AddPolicy(policy); err != nil {
			t.Fatal(err)
		}
	}
	for _, grouping := range [][]string{
		{"u_member", "role_a", "default"},
		{"u_admin", consts.AUTHZ_ROLE_ADMIN, "default"},
	} {
		if _, err := r.enforcer.AddGroupingPolicy(grouping); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.enforcer.AddNamedGroupingPolicy(
		systemRoleGrouping, "u_system", consts.AUTHZ_SYSTEM_ROLE_ROOT,
	); err != nil {
		t.Fatal(err)
	}
	return r
}
