package bigcache

import (
	"context"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

const (
	// shards must be a power of two, per the backend's requirement.
	shards = 16
	// lifeWindow is the single global lifetime every entry shares.
	lifeWindow = 10 * time.Minute
	// cleanWindow is how often the backend sweeps expired entries.
	cleanWindow = 5 * time.Minute
	// maxEntrySize is the backend's per-entry allocation hint in bytes.
	maxEntrySize = 64 * 1024
)

var store = registry.New()

type cache[T any] struct {
	c *bigcache.BigCache
}

// Cache returns the process-wide bigcache cache of type T, creating it on
// first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		c, err := bigcache.New(context.Background(), buildConfig())
		if err != nil {
			panic(err) // unreachable: the configuration is built from valid constants
		}
		return &cache[T]{c: c}
	})
}

// Set always returns ErrTTLNotSupported: entries in this backend expire on
// the global life window fixed at construction, so neither ttl == 0 (never
// expire) nor a per-entry ttl can be honored. The backend stays wired for the
// day the contract grows an opt-in for global expiration.
func (c *cache[T]) Set(context.Context, string, T, time.Duration) error {
	return types.ErrTTLNotSupported
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	var zero T
	val, err := c.c.Get(key)
	if err != nil {
		// The backend only fails a Get for a missing entry.
		return zero, types.ErrEntryNotFound
	}
	var result T
	if err := util.Unmarshal(val, &result); err != nil {
		return zero, err
	}
	return result, nil
}

func (c *cache[T]) Delete(_ context.Context, key string) error {
	if err := c.c.Delete(key); err != nil && !errors.Is(err, bigcache.ErrEntryNotFound) {
		return err
	}
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) bool {
	_, err := c.c.Get(key)
	return err == nil
}

func buildConfig() bigcache.Config {
	return bigcache.Config{
		Shards:       shards,
		LifeWindow:   lifeWindow,
		CleanWindow:  cleanWindow,
		MaxEntrySize: maxEntrySize,
	}
}
