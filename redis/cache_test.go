package redis_test

import (
	"testing"

	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/redis"
)

// TestCacheConformance runs the shared types.Cache conformance suite against
// the Redis backend, tracing wrapper included, on the testcontainer Redis
// this package's TestMain provisions.
func TestCacheConformance(t *testing.T) {
	cachetest.Run(t, redis.Cache[string](), cachetest.Capabilities{PerEntryTTL: true, NoExpiry: true})
}
