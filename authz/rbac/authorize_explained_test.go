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
// rule is withheld above: those branches satisfy the matcher against every
// stored row, so the engine reports whichever came first — here a row for an
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

func newExplainedFixture(t *testing.T) *rbac {
	t.Helper()

	r := benchRBAC(t, 0)
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
