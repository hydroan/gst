package freelru_test

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/cache/freelru"
	"github.com/hydroan/gst/types"
)

// writeVisibilityCapacity keeps the pressure tests' fill and warm-up cheap
// while still driving the cache to its bound.
const writeVisibilityCapacity = 2000

type (
	visibilityProbe string
	retentionProbe  string
)

func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if freelru.Cache[int]() != freelru.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}

func TestCacheIsolatesTypes(t *testing.T) {
	ctx := context.Background()
	if err := freelru.Cache[int]().Set(ctx, "package-isolation-key", 7, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := freelru.Cache[string]().Get(ctx, "package-isolation-key"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound from the other type's cache, got %v", err)
	}
}

// TestSetRejectsSubMillisecondTTL pins the granularity contract: a lifetime
// below the backend's millisecond resolution would truncate to expired on
// arrival, so it must be rejected instead of silently mis-honored.
func TestSetRejectsSubMillisecondTTL(t *testing.T) {
	ctx := context.Background()
	if err := freelru.Cache[int]().Set(ctx, "sub-ms-key", 7, 500*time.Microsecond); !errors.Is(err, types.ErrTTLNotSupported) {
		t.Fatalf("want ErrTTLNotSupported for a sub-millisecond ttl, got %v", err)
	}
	if err := freelru.Cache[int]().Set(ctx, "ms-key", 7, time.Millisecond); err != nil {
		t.Fatalf("want the smallest representable ttl to be accepted, got %v", err)
	}
}

func TestWriteVisibilityUnderCapacityPressure(t *testing.T) {
	old := config.App.Cache.MaxEntries
	config.App.Cache.MaxEntries = writeVisibilityCapacity
	defer func() { config.App.Cache.MaxEntries = old }()

	cachetest.RunWriteVisibility(t, freelru.Cache[visibilityProbe](), writeVisibilityCapacity)
}

func TestWriteRetentionUnderCapacityPressure(t *testing.T) {
	old := config.App.Cache.MaxEntries
	config.App.Cache.MaxEntries = writeVisibilityCapacity
	defer func() { config.App.Cache.MaxEntries = old }()

	cachetest.RunWriteRetention(t, freelru.Cache[retentionProbe](), writeVisibilityCapacity)
}
