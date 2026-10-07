package redis_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/types"
	gstredis "github.com/hydroan/gst/redis"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestCacheConformance runs the shared types.Cache conformance suite against
// the Redis backend, tracing wrapper included, on the testcontainer Redis
// this package's TestMain provisions.
func TestCacheConformance(t *testing.T) {
	cachetest.Run(t, gstredis.Cache[string](), cachetest.Capabilities{PerEntryTTL: true, NoExpiry: true})
}

type cacheSample struct {
	Name      string    `json:"name"`
	Num       int       `json:"num"`
	CreatedAt time.Time `json:"created_at"`
}

func TestCacheStructRoundtrip(t *testing.T) {
	ctx := context.Background()
	c := gstredis.Cache[cacheSample]()
	want := cacheSample{Name: "roundtrip", Num: 42, CreatedAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)}
	if err := c.Set(ctx, "cache-test:struct", want, time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := c.Get(ctx, "cache-test:struct")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.Name != want.Name || got.Num != want.Num {
		t.Fatalf("want %+v, got %+v", want, got)
	}
}

// TestCachePointerRoundtrip pins the pointer-value behavior, including that a
// stored nil pointer serializes as JSON null and reads back as (nil, nil).
func TestCachePointerRoundtrip(t *testing.T) {
	ctx := context.Background()
	c := gstredis.Cache[*cacheSample]()

	want := &cacheSample{Name: "pointer", Num: 7}
	if err := c.Set(ctx, "cache-test:pointer", want, time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := c.Get(ctx, "cache-test:pointer")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.Name != want.Name || got.Num != want.Num {
		t.Fatalf("want %+v, got %+v", want, got)
	}

	if err = c.Set(ctx, "cache-test:nil-pointer", nil, time.Minute); err != nil {
		t.Fatalf("set nil pointer: %v", err)
	}
	got, err = c.Get(ctx, "cache-test:nil-pointer")
	if err != nil {
		t.Fatalf("get nil pointer: %v", err)
	}
	if got != nil {
		t.Fatalf("want nil pointer back, got %+v", got)
	}
}

func TestCacheLargeValueRoundtrip(t *testing.T) {
	ctx := context.Background()
	c := gstredis.Cache[string]()
	want := strings.Repeat("x", 1<<20) // 1MB
	if err := c.Set(ctx, "cache-test:large", want, time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := c.Get(ctx, "cache-test:large")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != want {
		t.Fatalf("large value corrupted: want %d bytes, got %d", len(want), len(got))
	}
}

// TestCacheSetOverwriteResetsTTL pins the overwrite semantics: a second Set
// replaces both the value and the ttl, so overwriting with ttl == 0 makes the
// entry outlive the first short ttl.
func TestCacheSetOverwriteResetsTTL(t *testing.T) {
	ctx := context.Background()
	c := gstredis.Cache[string]()
	if err := c.Set(ctx, "cache-test:overwrite", "short-lived", 300*time.Millisecond); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if err := c.Set(ctx, "cache-test:overwrite", "kept", 0); err != nil {
		t.Fatalf("second set: %v", err)
	}

	time.Sleep(600 * time.Millisecond)
	got, err := c.Get(ctx, "cache-test:overwrite")
	if err != nil {
		t.Fatalf("get after the first ttl passed: %v", err)
	}
	if got != "kept" {
		t.Fatalf("want %q, got %q", "kept", got)
	}
}

// TestCacheKeyspaceIsSharedAcrossTypes pins the documented contract: unlike
// the in-memory backends, gstredis.Cache handles of different types share one
// keyspace, so key isolation belongs to the caller's key builders. A handle of
// another type reads the same entry whenever its bytes decode as that type.
func TestCacheKeyspaceIsSharedAcrossTypes(t *testing.T) {
	ctx := context.Background()
	if err := gstredis.Cache[int]().Set(ctx, "cache-test:shared-keyspace", 42, time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := gstredis.Cache[int64]().Get(ctx, "cache-test:shared-keyspace")
	if err != nil {
		t.Fatalf("want the other type's handle to read the shared entry, got %v", err)
	}
	if got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
}

// TestCacheDropsAnEntryItCannotDecode pins the read contract for an entry
// whose bytes do not decode as T, here a string read through an int handle of
// the shared keyspace: Get answers ErrEntryNotFound like a plain miss, drops
// the entry so the next read is a plain miss, and logs the key once at Warn.
func TestCacheDropsAnEntryItCannotDecode(t *testing.T) {
	ctx := context.Background()
	if err := gstredis.Cache[string]().Set(ctx, "cache-test:unreadable", "text", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	core, logs := observer.New(zap.WarnLevel)
	t.Cleanup(zap.ReplaceGlobals(zap.New(core)))

	_, err := gstredis.Cache[int]().Get(ctx, "cache-test:unreadable")
	if !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound for an entry that cannot be decoded, got %v", err)
	}
	if gstredis.Cache[string]().Exists(ctx, "cache-test:unreadable") {
		t.Fatal("want the unreadable entry dropped from the shared keyspace")
	}
	if entries := logs.FilterField(zap.String("key", "cache-test:unreadable")).All(); len(entries) != 1 {
		t.Fatalf("want one warning naming the dropped key, got %d in %+v", len(entries), logs.All())
	}
}
