package redis_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/types"
)

// TestCacheConformance runs the shared types.Cache conformance suite against
// the Redis backend, tracing wrapper included, on the testcontainer Redis
// this package's TestMain provisions.
func TestCacheConformance(t *testing.T) {
	cachetest.Run(t, redis.Cache[string](), cachetest.Capabilities{PerEntryTTL: true, NoExpiry: true})
}

type cacheSample struct {
	Name      string    `json:"name"`
	Num       int       `json:"num"`
	CreatedAt time.Time `json:"created_at"`
}

func TestCacheStructRoundtrip(t *testing.T) {
	ctx := context.Background()
	c := redis.Cache[cacheSample]()
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
	c := redis.Cache[*cacheSample]()

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
	c := redis.Cache[string]()
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
	c := redis.Cache[string]()
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
// the in-memory backends, redis.Cache handles of different types share one
// keyspace, so key isolation belongs to the caller's key builders. A cache of
// another type sees the raw bytes and fails to decode them instead of
// answering ErrEntryNotFound.
func TestCacheKeyspaceIsSharedAcrossTypes(t *testing.T) {
	ctx := context.Background()
	if err := redis.Cache[string]().Set(ctx, "cache-test:shared-keyspace", "text", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}

	_, err := redis.Cache[int]().Get(ctx, "cache-test:shared-keyspace")
	if err == nil {
		t.Fatal("want a decode error when reading another type's entry")
	}
	if errors.Is(err, types.ErrEntryNotFound) {
		t.Fatal("want a shared keyspace: ErrEntryNotFound would mean the types are isolated")
	}
}
