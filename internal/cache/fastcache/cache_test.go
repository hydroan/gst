package fastcache_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/fastcache"
	"github.com/hydroan/gst/types"
)

type sample struct {
	Name string `json:"name"`
	Num  int    `json:"num"`
}

func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if fastcache.Cache[int]() != fastcache.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}

func TestCacheIsolatesTypes(t *testing.T) {
	ctx := context.Background()
	if err := fastcache.Cache[int]().Set(ctx, "package-isolation-key", 7, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := fastcache.Cache[string]().Get(ctx, "package-isolation-key"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound from the other type's cache, got %v", err)
	}
}

func TestStructRoundtrip(t *testing.T) {
	ctx := context.Background()
	c := fastcache.Cache[sample]()
	want := sample{Name: "roundtrip", Num: 42}
	if err := c.Set(ctx, "struct-roundtrip", want, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := c.Get(ctx, "struct-roundtrip")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != want {
		t.Fatalf("want %+v, got %+v", want, got)
	}
}

// TestSetRejectsOversizedEntry pins the explicit error for entries above the
// backend's 64KB chunk limit; the backend itself would drop them silently.
func TestSetRejectsOversizedEntry(t *testing.T) {
	ctx := context.Background()
	c := fastcache.Cache[string]()
	oversized := strings.Repeat("x", 70*1024)
	if err := c.Set(ctx, "oversized-entry", oversized, 0); err == nil {
		t.Fatal("want error for entry above the chunk limit")
	}
	if _, err := c.Get(ctx, "oversized-entry"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound after rejected set, got %v", err)
	}
}
