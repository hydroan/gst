package ccache_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/cache/ccache"
	"github.com/hydroan/gst/types"
)

// writeVisibilityCapacity keeps the pressure test's fill and warm-up cheap
// while still driving the cache to its bound.
const writeVisibilityCapacity = 2000

// visibilityProbe gets its own per-type instance, so the pressure test can
// size and fill a cache without disturbing the shared ones.
type visibilityProbe string

// TestWriteVisibilityUnderCapacityPressure guards the property that decided
// this backend's promotion to the forwarded default: writes stay visible when
// the cache is full and warm.
func TestWriteVisibilityUnderCapacityPressure(t *testing.T) {
	old := config.App.Cache.MaxEntries
	config.App.Cache.MaxEntries = writeVisibilityCapacity
	defer func() { config.App.Cache.MaxEntries = old }()

	cachetest.RunWriteVisibility(t, ccache.Cache[visibilityProbe](), writeVisibilityCapacity)
}

// TestWriteRetentionUnderCapacityPressure guards the other half of the same
// property: entries written under pressure survive until a later request, not
// just until the next line. The forwarded default has to hold this, and the
// scan-resistant backends do not.
func TestWriteRetentionUnderCapacityPressure(t *testing.T) {
	old := config.App.Cache.MaxEntries
	config.App.Cache.MaxEntries = writeVisibilityCapacity
	defer func() { config.App.Cache.MaxEntries = old }()

	cachetest.RunWriteRetention(t, ccache.Cache[retentionProbe](), writeVisibilityCapacity)
}

// retentionProbe gets its own per-type instance for the same reason
// visibilityProbe does.
type retentionProbe string

// TestSizedValueDoesNotShrinkCapacity guards the boxing in the wrapper: the
// backend charges a value's own Size() against MaxSize when the value
// implements its Sized interface, which would turn the entry bound into a
// cost budget for any caller type carrying that method.
func TestSizedValueDoesNotShrinkCapacity(t *testing.T) {
	ctx := context.Background()
	c := ccache.Cache[sizedValue]()
	const writes = 500

	for i := range writes {
		if err := c.Set(ctx, "sized-"+strconv.Itoa(i), sizedValue{}, 0); err != nil {
			t.Fatalf("set: %v", err)
		}
	}
	var resident int
	for i := range writes {
		if c.Exists(ctx, "sized-"+strconv.Itoa(i)) {
			resident++
		}
	}
	// Every entry costs one, so a bound far above the write count evicts
	// nothing. Without the boxing each entry would charge 10000 instead.
	if resident != writes {
		t.Fatalf("want all %d entries resident, got %d — the entry bound became a cost budget", writes, resident)
	}
}

// sizedValue carries the method that the backend's Sized interface matches.
type sizedValue struct{}

func (sizedValue) Size() int64 { return 10000 }

func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if ccache.Cache[int]() != ccache.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}

func TestCacheIsolatesTypes(t *testing.T) {
	ctx := context.Background()
	if err := ccache.Cache[int]().Set(ctx, "package-isolation-key", 7, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := ccache.Cache[string]().Get(ctx, "package-isolation-key"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound from the other type's cache, got %v", err)
	}
}
