package rbac

import (
	"context"
	"testing"

	"github.com/hydroan/gst/types/consts"
)

// BenchmarkAuthorize measures a decision on the branch that costs the most: a
// role grant, which reaches the source derivation only after both membership
// lookups miss.
func BenchmarkAuthorize(b *testing.B) {
	r := newTestRBAC(b, 363)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		decision, err := r.Authorize(ctx, "default", "u1", "/api/things/300", "GET")
		source := decision.Source
		if err != nil {
			b.Fatal(err)
		}
		if source != consts.GrantSourceRole {
			b.Fatalf("expected role source, got %q", source)
		}
	}
}

// BenchmarkAuthorizeSystemRoot covers the branch that no longer reaches the
// engine at all. It is sized like the others so the comparison shows what
// deciding a branch outside the matcher is worth: this one answers from the
// role graph, so its cost should not follow the size of the policy set.
func BenchmarkAuthorizeSystemRoot(b *testing.B) {
	r := newTestRBAC(b, 363)
	seed(b, systemRoleGrouping, []string{"u_root", consts.AUTHZ_SYSTEM_ROLE_ROOT})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		decision, err := r.Authorize(ctx, "default", "u_root", "/api/things/300", "GET")
		allowed := decision.Allowed
		if err != nil || !allowed {
			b.Fatalf("allowed=%v err=%v", allowed, err)
		}
	}
}

// BenchmarkAuthorizeDenied covers the denial path, which skips the
// derivation entirely.
func BenchmarkAuthorizeDenied(b *testing.B) {
	r := newTestRBAC(b, 363)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		decision, err := r.Authorize(ctx, "default", "u1", "/api/nope", "GET")
		allowed := decision.Allowed
		if err != nil {
			b.Fatal(err)
		}
		if allowed {
			b.Fatal("expected denial")
		}
	}
}
