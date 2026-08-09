package bigcache

import (
	"context"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/codec"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
)

const (
	// shards must be a power of two, per the backend's requirement.
	shards = 16

	// lifeWindow is the backend's only expiry control and it applies to every
	// entry alike, so it is set past any process lifetime: the contract's
	// ttl 0 means the entry does not expire, and a shorter global window
	// would quietly break that promise. Per-entry lifetimes are rejected by
	// Set instead.
	lifeWindow = 100 * 365 * 24 * time.Hour

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

// Set stores the value under key. The backend expires entries on one global
// window rather than per entry, so only ttl == 0 is accepted.
func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	if ttl > 0 {
		return types.ErrTTLNotSupported
	}
	val, err := codec.Marshal(value)
	if err != nil {
		return err
	}
	return c.c.Set(key, val)
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	var zero T
	val, err := c.c.Get(key)
	if err != nil {
		// The backend only fails a Get for a missing entry.
		return zero, types.ErrEntryNotFound
	}
	var result T
	if err := codec.Unmarshal(val, &result); err != nil {
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
		MaxEntrySize: maxEntrySize,
	}
}
