package lru_test

import (
	"context"
	"os"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/cachetest"
	"github.com/hydroan/gst/internal/cache/lru"
	"github.com/hydroan/gst/types"
)

func TestMain(m *testing.M) {
	cachetest.FillTestConfig()
	os.Exit(m.Run())
}

func TestCacheReturnsSameInstancePerType(t *testing.T) {
	if lru.Cache[int]() != lru.Cache[int]() {
		t.Fatal("want the same instance for one type")
	}
}

func TestCacheIsolatesTypes(t *testing.T) {
	ctx := context.Background()
	if err := lru.Cache[int]().Set(ctx, "package-isolation-key", 7, 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := lru.Cache[string]().Get(ctx, "package-isolation-key"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound from the other type's cache, got %v", err)
	}
}

func TestInitRejectsInvalidCapacity(t *testing.T) {
	old := config.App.Cache.Capacity
	config.App.Cache.Capacity = 0
	defer func() { config.App.Cache.Capacity = old }()

	if err := lru.Init(); err == nil {
		t.Fatal("want error for zero capacity")
	}
}

func TestCachePanicsWithInvalidConfig(t *testing.T) {
	old := config.App.Cache.Capacity
	config.App.Cache.Capacity = 0
	defer func() { config.App.Cache.Capacity = old }()
	defer func() {
		if recover() == nil {
			t.Fatal("want panic for invalid configuration")
		}
	}()

	// A fresh probe type dodges the per-type singleton, so construction runs.
	type invalidConfigProbe struct{ _ byte }
	_ = lru.Cache[invalidConfigProbe]()
}
