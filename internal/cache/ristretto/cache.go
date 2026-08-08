package ristretto

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
)

// defaultMaxEntries bounds every per-type cache when the cache configuration
// does not override it. Instances are created lazily per type, so only used
// types pay the admission-counter cost of roughly 4MB per instance at this
// capacity.
const defaultMaxEntries = 100_000

var store = registry.New()

type cache[T any] struct {
	c *ristretto.Cache[string, T]
}

// Cache returns the process-wide ristretto cache of type T, creating it on
// first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		c, err := ristretto.NewCache(buildConf[T]())
		if err != nil {
			panic(err) // unreachable: the configuration is built from positive constants
		}
		return &cache[T]{c: c}
	})
}

func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	if success := c.c.SetWithTTL(key, value, 1, ttl); !success {
		return errors.New("cache rejected the set operation")
	}
	// Block until the buffered set is applied so a Set followed by a Get on
	// the same key observes the value (read-your-write).
	c.c.Wait()
	return nil
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	value, ok := c.c.Get(key)
	if !ok {
		var zero T
		return zero, types.ErrEntryNotFound
	}
	return value, nil
}

func (c *cache[T]) Delete(_ context.Context, key string) error {
	c.c.Del(key)
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) bool {
	_, exists := c.c.Get(key)
	return exists
}

// buildConf sizes the cache following the backend's own guidance: MaxCost
// bounds the entry count because every entry costs 1, NumCounters is ten
// times the expected entries to keep the admission sketch accurate, and
// BufferItems is the recommended write-buffer size.
func buildConf[T any]() *ristretto.Config[string, T] {
	entries := int64(maxEntries())
	return &ristretto.Config[string, T]{
		NumCounters: entries * 10,
		MaxCost:     entries,
		BufferItems: 64,
		// Every entry costs exactly 1 so MaxCost bounds the entry count; the
		// backend would otherwise add its per-item bookkeeping bytes to each
		// cost and silently shrink the capacity about 57-fold.
		IgnoreInternalCost: true,
	}
}

// maxEntries returns the configured per-type entry bound, falling back to
// the built-in default when it is unset or the configuration is not loaded
// yet.
func maxEntries() int {
	if v := config.App.Cache.MaxEntries; v > 0 {
		return v
	}
	return defaultMaxEntries
}
