package cachetest_test

import (
	"os"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/bigcache"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/cache/ccache"
	"github.com/hydroan/gst/internal/cache/cmap"
	"github.com/hydroan/gst/internal/cache/fastcache"
	"github.com/hydroan/gst/internal/cache/freecache"
	"github.com/hydroan/gst/internal/cache/gocache"
	"github.com/hydroan/gst/internal/cache/lru"
	"github.com/hydroan/gst/internal/cache/lrue"
	"github.com/hydroan/gst/internal/cache/ristretto"
	"github.com/hydroan/gst/internal/cache/smap"
)

func TestMain(m *testing.M) {
	// The backends read the cache configuration at construction; fill it
	// directly instead of running the full config bootstrap.
	config.App.Cache.Capacity = 100000
	config.App.Cache.Expiration = 10 * time.Minute
	config.App.Cache.CleanWindow = 5 * time.Minute
	config.App.Cache.LifeWindow = 10 * time.Minute
	config.App.Cache.Shards = 16
	os.Exit(m.Run())
}

func TestConformance(t *testing.T) {
	t.Run("ristretto", func(t *testing.T) {
		cachetest.Run(t, ristretto.Cache[string](), cachetest.Capabilities{PerEntryTTL: true, NoExpiry: true})
	})
	t.Run("freecache", func(t *testing.T) {
		cachetest.Run(t, freecache.Cache[string](), cachetest.Capabilities{PerEntryTTL: true, NoExpiry: true, TTLGranularity: time.Second})
	})
	t.Run("gocache", func(t *testing.T) {
		cachetest.Run(t, gocache.Cache[string](), cachetest.Capabilities{PerEntryTTL: true, NoExpiry: true})
	})
	t.Run("ccache", func(t *testing.T) {
		cachetest.Run(t, ccache.Cache[string](), cachetest.Capabilities{PerEntryTTL: true, NoExpiry: true})
	})
	t.Run("lru", func(t *testing.T) {
		cachetest.Run(t, lru.Cache[string](), cachetest.Capabilities{NoExpiry: true})
	})
	t.Run("cmap", func(t *testing.T) {
		cachetest.Run(t, cmap.Cache[string](), cachetest.Capabilities{NoExpiry: true})
	})
	t.Run("smap", func(t *testing.T) {
		cachetest.Run(t, smap.Cache[string](), cachetest.Capabilities{NoExpiry: true})
	})
	t.Run("fastcache", func(t *testing.T) {
		cachetest.Run(t, fastcache.Cache[string](), cachetest.Capabilities{NoExpiry: true})
	})
	t.Run("lrue", func(t *testing.T) {
		cachetest.Run(t, lrue.Cache[string](), cachetest.Capabilities{})
	})
	t.Run("bigcache", func(t *testing.T) {
		cachetest.Run(t, bigcache.Cache[string](), cachetest.Capabilities{})
	})
}
