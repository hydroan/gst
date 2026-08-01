package rbac

import (
	"context"
	"testing"

	"github.com/hydroan/gst/types/consts"
)

// BenchmarkAuthorize measures the existing decision-only path.
func BenchmarkAuthorize(b *testing.B) {
	r := newTestRBAC(b, 363)
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
	r := newTestRBAC(b, 363)
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
	r := newTestRBAC(b, 363)
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
