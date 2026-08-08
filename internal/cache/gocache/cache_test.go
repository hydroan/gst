package gocache_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/cache/gocache"
	"github.com/hydroan/gst/types"
)

func TestMain(m *testing.M) {
	cachetest.FillTestConfig()
	os.Exit(m.Run())
}

func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if gocache.Cache[int]() != gocache.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}

func TestCacheIsolatesTypes(t *testing.T) {
	ctx := context.Background()
	if err := gocache.Cache[int]().Set(ctx, "package-isolation-key", 7, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := gocache.Cache[string]().Get(ctx, "package-isolation-key"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound from the other type's cache, got %v", err)
	}
}

// TestZeroTTLOutlivesDefaultExpiration pins the ttl mapping this backend used
// to get wrong: the backend treats a zero duration as "use the default
// expiration", so Set must translate the contract's ttl == 0 to the explicit
// no-expiration marker. A short default expiration on a fresh backend makes
// the difference observable.
func TestZeroTTLOutlivesDefaultExpiration(t *testing.T) {
	old := config.App.Cache.Expiration
	config.App.Cache.Expiration = 200 * time.Millisecond
	defer func() { config.App.Cache.Expiration = old }()

	// A fresh probe type gets a fresh backend built with the short default.
	type zeroTTLProbe struct{ Name string }
	c := gocache.Cache[zeroTTLProbe]()
	ctx := context.Background()
	if err := c.Set(ctx, "zero-ttl-key", zeroTTLProbe{Name: "keep"}, 0); err != nil {
		t.Fatalf("set: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	if _, err := c.Get(ctx, "zero-ttl-key"); err != nil {
		t.Fatalf("ttl=0 entry must outlive the default expiration: %v", err)
	}
}
