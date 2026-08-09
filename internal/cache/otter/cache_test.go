package otter_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/cache/otter"
	"github.com/hydroan/gst/types"
)

// writeVisibilityCapacity keeps the pressure test's fill and warm-up cheap
// while still driving the cache to its bound.
const writeVisibilityCapacity = 2000

// visibilityProbe gets its own per-type instance, so the pressure test can
// size and fill a cache without disturbing the shared ones.
type visibilityProbe string

func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if otter.Cache[int]() != otter.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}

func TestCacheIsolatesTypes(t *testing.T) {
	ctx := context.Background()
	if err := otter.Cache[int]().Set(ctx, "package-isolation-key", 7, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := otter.Cache[string]().Get(ctx, "package-isolation-key"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound from the other type's cache, got %v", err)
	}
}

// TestWriteVisibilityUnderCapacityPressure guards the property that makes
// this backend a candidate for the forwarded default: writes stay visible
// when the cache is full and warm.
func TestWriteVisibilityUnderCapacityPressure(t *testing.T) {
	old := config.App.Cache.MaxEntries
	config.App.Cache.MaxEntries = writeVisibilityCapacity
	defer func() { config.App.Cache.MaxEntries = old }()

	cachetest.RunWriteVisibility(t, otter.Cache[visibilityProbe](), writeVisibilityCapacity)
}

// TestOverwriteReappliesTTL pins the per-write lifetime: the backend expires
// per cache, so the wrapper carries each call's ttl with the value. An
// overwrite must therefore replace the previous deadline in both directions.
func TestOverwriteReappliesTTL(t *testing.T) {
	ctx := context.Background()
	c := otter.Cache[ttlProbe]()

	// Permanent, then shortened: the entry must expire.
	if err := c.Set(ctx, "shortened", "v", 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := c.Set(ctx, "shortened", "v", 200*time.Millisecond); err != nil {
		t.Fatalf("set: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := c.Get(ctx, "shortened"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want the shortened ttl to win, got %v", err)
	}

	// Short, then permanent: the entry must survive.
	if err := c.Set(ctx, "extended", "v", 200*time.Millisecond); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := c.Set(ctx, "extended", "v", 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := c.Get(ctx, "extended"); err != nil {
		t.Fatalf("want the permanent ttl to win: %v", err)
	}
}

// TestReadDoesNotExtendTTL pins the expiry basis: ttl counts from the write,
// so reading an entry must not keep it alive past its lifetime.
func TestReadDoesNotExtendTTL(t *testing.T) {
	ctx := context.Background()
	c := otter.Cache[ttlProbe]()

	if err := c.Set(ctx, "read-probe", "v", 300*time.Millisecond); err != nil {
		t.Fatalf("set: %v", err)
	}
	for range 5 {
		time.Sleep(80 * time.Millisecond)
		_, _ = c.Get(ctx, "read-probe")
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := c.Get(ctx, "read-probe"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want the entry to expire despite the reads, got %v", err)
	}
}

// ttlProbe keeps the ttl tests on their own instance so their sleeps do not
// interact with the other tests' entries.
type ttlProbe string

// TestScanResistanceUpToCapacity records why this backend is carried: it
// admits every write and still keeps a hot working set through a sweep of
// roughly its own size, where a plain-LRU backend keeps none of it. The bar
// sits far below the measured survival (198 of 200) because the property is
// statistical, and the sweep stays at one times the capacity because a
// several-times sweep displaces this backend too — admission buys a bounded
// amount of resistance, not immunity.
func TestScanResistanceUpToCapacity(t *testing.T) {
	ctx := context.Background()
	c := otter.Cache[scanProbe]()

	const hot = 200
	for i := range hot {
		if err := c.Set(ctx, "hot-"+strconv.Itoa(i), "v", 0); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	for range 20 {
		for i := range hot {
			_, _ = c.Get(ctx, "hot-"+strconv.Itoa(i))
		}
	}

	for i := range defaultCapacity {
		_ = c.Set(ctx, "scan-"+strconv.Itoa(i), "v", 0)
	}

	var survived int
	for i := range hot {
		if c.Exists(ctx, "hot-"+strconv.Itoa(i)) {
			survived++
		}
	}
	if survived < hot/2 {
		t.Fatalf("want the hot set to survive a sweep of one capacity, only %d of %d left", survived, hot)
	}
}

type scanProbe string

// defaultCapacity mirrors the backend's built-in bound, which the scan test
// runs against because it does not override the configuration.
const defaultCapacity = 100_000
