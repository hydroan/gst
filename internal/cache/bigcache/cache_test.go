package bigcache_test

import (
	"testing"

	"github.com/hydroan/gst/internal/cache/bigcache"
)

// TestCacheReturnsSameInstancePerType covers the singleton; the ttl contract
// behavior (Set always returns ErrTTLNotSupported) is covered by the shared
// conformance suite.
func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if bigcache.Cache[int]() != bigcache.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}
