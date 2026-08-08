package bigcache_test

import (
	"os"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/bigcache"
	"github.com/hydroan/gst/internal/cache/cachetest"
)

func TestMain(m *testing.M) {
	cachetest.FillTestConfig()
	os.Exit(m.Run())
}

// TestCacheReturnsSameInstancePerType covers the singleton; the ttl contract
// behavior (Set always returns ErrTTLNotSupported) is covered by the shared
// conformance suite.
func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if bigcache.Cache[int]() != bigcache.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}

func TestInitRejectsInvalidShards(t *testing.T) {
	old := config.App.Cache.Shards
	config.App.Cache.Shards = 3 // the backend requires a power of two
	defer func() { config.App.Cache.Shards = old }()

	if err := bigcache.Init(); err == nil {
		t.Fatal("want error for a shard count that is not a power of two")
	}
}

func TestCachePanicsWithInvalidConfig(t *testing.T) {
	old := config.App.Cache.Shards
	config.App.Cache.Shards = 3
	defer func() { config.App.Cache.Shards = old }()
	defer func() {
		if recover() == nil {
			t.Fatal("want panic for invalid configuration")
		}
	}()

	// A fresh probe type dodges the per-type singleton, so construction runs.
	type invalidConfigProbe struct{ _ byte }
	_ = bigcache.Cache[invalidConfigProbe]()
}
