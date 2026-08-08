package cmap_test

import (
	"context"
	"os"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/cache/cmap"
	"github.com/hydroan/gst/types"
)

func TestMain(m *testing.M) {
	cachetest.FillTestConfig()
	os.Exit(m.Run())
}

func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if cmap.Cache[int]() != cmap.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}

func TestCacheIsolatesTypes(t *testing.T) {
	ctx := context.Background()
	if err := cmap.Cache[int]().Set(ctx, "package-isolation-key", 7, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := cmap.Cache[string]().Get(ctx, "package-isolation-key"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound from the other type's cache, got %v", err)
	}
}
