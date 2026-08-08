package lrue_test

import (
	"testing"

	"github.com/hydroan/gst/internal/cache/lrue"
)

// TestCacheReturnsSameInstancePerType is the only backend-specific check: the
// ttl contract behavior (Set always returns ErrTTLNotSupported) is covered by
// the shared conformance suite.
func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if lrue.Cache[int]() != lrue.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}
