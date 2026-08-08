package lrue_test

import (
	"os"
	"testing"

	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/cache/lrue"
)

func TestMain(m *testing.M) {
	cachetest.FillTestConfig()
	os.Exit(m.Run())
}

// TestCacheReturnsSameInstancePerType is the only backend-specific check: the
// ttl contract behavior (Set always returns ErrTTLNotSupported) is covered by
// the shared conformance suite.
func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if lrue.Cache[int]() != lrue.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}
