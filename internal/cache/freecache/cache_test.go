package freecache_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/cache/freecache"
	"github.com/hydroan/gst/types"
)

func TestMain(m *testing.M) {
	cachetest.FillTestConfig()
	os.Exit(m.Run())
}

type sample struct {
	Name string `json:"name"`
	Num  int    `json:"num"`
}

func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if freecache.Cache[int]() != freecache.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}

func TestCacheIsolatesTypes(t *testing.T) {
	ctx := context.Background()
	if err := freecache.Cache[int]().Set(ctx, "package-isolation-key", 7, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := freecache.Cache[string]().Get(ctx, "package-isolation-key"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound from the other type's cache, got %v", err)
	}
}

func TestStructRoundtrip(t *testing.T) {
	ctx := context.Background()
	c := freecache.Cache[sample]()
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

// TestSetRejectsEntryAboveBackendLimit records the backend's per-entry
// ceiling: freecache caps one entry at 1/1024 of the cache size, and the
// cache size bottoms out at 512KB, so with the test capacity a 1KB value is
// rejected with an explicit error rather than stored partially.
func TestSetRejectsEntryAboveBackendLimit(t *testing.T) {
	ctx := context.Background()
	c := freecache.Cache[string]()
	oversized := strings.Repeat("x", 1024)
	if err := c.Set(ctx, "oversized-entry", oversized, 0); err == nil {
		t.Fatal("want error for entry above the backend per-entry limit")
	}
	if _, err := c.Get(ctx, "oversized-entry"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound after rejected set, got %v", err)
	}
}
