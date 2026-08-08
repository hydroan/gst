package bigcache

import (
	"context"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

var (
	store = registry.New()

	maxEntrySize     = 1024 * 64 // 64KB
	hardMaxCacheSize = 0
	verbose          = false
)

func Init() error {
	tmp, err := bigcache.New(context.Background(), buildConfig())
	if err != nil {
		return err
	}
	return tmp.Close()
}

type cache[T any] struct {
	c *bigcache.BigCache
}

// Cache returns the process-wide bigcache cache of type T, creating it on
// first use. Creation failures panic: they mean the cache configuration was
// never loaded or is invalid, which Init reports as an error during bootstrap.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		c, err := bigcache.New(context.Background(), buildConfig())
		if err != nil {
			panic(errors.Wrap(err, "bigcache: create cache (run config.Init and cache.Init before requesting caches)"))
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
		Shards:           config.App.Shards,
		LifeWindow:       config.App.Cache.LifeWindow,
		CleanWindow:      config.App.Cache.CleanWindow,
		MaxEntrySize:     maxEntrySize,
		HardMaxCacheSize: hardMaxCacheSize,
		Verbose:          verbose,
	}
}
