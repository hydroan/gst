package requestctx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types/consts"
)

// BenchmarkRequestMetadataWithoutParams measures the per-request metadata cost
// of a route declaring no parameters and receiving no query string — the shape
// of most read endpoints, and the one where every map allocated is an empty one.
func BenchmarkRequestMetadataWithoutParams(b *testing.B) {
	benchmarkRequestMetadata(b, "/bench", "/bench", nil)
}

// BenchmarkRequestMetadataWithParams measures the same path for a route that
// does carry a parameter and a query string, where the maps hold something.
func BenchmarkRequestMetadataWithParams(b *testing.B) {
	benchmarkRequestMetadata(b, "/bench/:id", "/bench/42?page=1&size=20", []string{"id"})
}

// benchmarkRequestMetadata times WithMetadata(FromGin(c)), the pair every
// request context is built from. The result is discarded: b.Loop keeps call
// arguments and results alive, so the loop body survives the optimizer.
//
// The loop runs inside the handler because gin returns the context to its pool
// once ServeHTTP returns; a context captured and used afterwards is one the
// framework already considers free.
func benchmarkRequestMetadata(b *testing.B, route, target string, paramKeys []string) {
	b.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET(route, func(c *gin.Context) {
		if len(paramKeys) > 0 {
			c.Set(consts.PARAMS, paramKeys)
		}

		base := context.Background()
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			WithMetadata(base, FromGin(c))
		}
		b.StopTimer()
	})

	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
}
