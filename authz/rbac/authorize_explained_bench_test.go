package rbac

import (
	"context"
	"fmt"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/hydroan/gst/types/consts"
)

// nullContextAdapter satisfies the adapter a ContextEnforcer requires while
// keeping the benchmark entirely in memory: policies are added through the
// enforcer and never persisted anywhere.
type nullContextAdapter struct{}

func (*nullContextAdapter) LoadPolicyCtx(context.Context, casbinmodel.Model) error { return nil }
func (*nullContextAdapter) SavePolicyCtx(context.Context, casbinmodel.Model) error { return nil }
func (*nullContextAdapter) AddPolicyCtx(context.Context, string, string, []string) error {
	return nil
}

func (*nullContextAdapter) RemovePolicyCtx(context.Context, string, string, []string) error {
	return nil
}

func (*nullContextAdapter) RemoveFilteredPolicyCtx(
	context.Context, string, string, int, ...string,
) error {
	return nil
}

func (*nullContextAdapter) LoadPolicy(casbinmodel.Model) error          { return nil }
func (*nullContextAdapter) SavePolicy(casbinmodel.Model) error          { return nil }
func (*nullContextAdapter) AddPolicy(string, string, []string) error    { return nil }
func (*nullContextAdapter) RemovePolicy(string, string, []string) error { return nil }
func (*nullContextAdapter) RemoveFilteredPolicy(string, string, int, ...string) error {
	return nil
}

// benchEnforcer builds an in-memory enforcer holding policyCount role policies,
// sized after a realistic deployment so the comparison reflects the per-policy
// matcher evaluation that dominates the cost.
func benchEnforcer(tb testing.TB, policyCount int) *casbin.ContextEnforcer {
	tb.Helper()

	m, err := casbinmodel.NewModelFromString(string(modelData))
	if err != nil {
		tb.Fatal(err)
	}
	ctxEnforcer, err := casbin.NewContextEnforcer(m, new(nullContextAdapter))
	if err != nil {
		tb.Fatal(err)
	}
	e, ok := ctxEnforcer.(*casbin.ContextEnforcer)
	if !ok {
		tb.Fatal("expected a context enforcer")
	}
	for i := range policyCount {
		if _, err = e.AddPolicy("default", "role_a", fmt.Sprintf("/api/things/%d", i), "GET", "allow"); err != nil {
			tb.Fatal(err)
		}
	}
	if _, err = e.AddGroupingPolicy("u1", "role_a", "default"); err != nil {
		tb.Fatal(err)
	}
	return e
}

func benchRBAC(tb testing.TB, policyCount int) *rbac {
	tb.Helper()
	return &rbac{enforcer: benchEnforcer(tb, policyCount), mu: &enforcerMu}
}

// BenchmarkAuthorize measures the existing decision-only path.
func BenchmarkAuthorize(b *testing.B) {
	r := benchRBAC(b, 363)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := r.Authorize(ctx, "default", "u1", "/api/things/300", "GET"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAuthorizeExplained measures the same decision plus the explanation,
// on the branch that costs the most: a role grant, which reaches the source
// derivation only after both membership lookups miss.
func BenchmarkAuthorizeExplained(b *testing.B) {
	r := benchRBAC(b, 363)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, source, _, err := r.AuthorizeExplained(ctx, "default", "u1", "/api/things/300", "GET")
		if err != nil {
			b.Fatal(err)
		}
		if source != consts.GrantSourceRole {
			b.Fatalf("expected role source, got %q", source)
		}
	}
}

// BenchmarkAuthorizeExplainedDenied covers the denial path, which skips the
// derivation entirely.
func BenchmarkAuthorizeExplainedDenied(b *testing.B) {
	r := benchRBAC(b, 363)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		allowed, _, _, err := r.AuthorizeExplained(ctx, "default", "u1", "/api/nope", "GET")
		if err != nil {
			b.Fatal(err)
		}
		if allowed {
			b.Fatal("expected denial")
		}
	}
}
