package lru

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
)

var store = registry.New()

func Init() error {
	tmp, err := lru.New[string, any](config.App.Cache.Capacity)
	if err != nil {
		return err
	}
	tmp.Purge()
	return nil
}

type cache[T any] struct {
	c *lru.Cache[string, T]
}

// Cache returns the process-wide lru cache of type T, creating it on first
// use. Creation failures panic: they mean the cache configuration was never
// loaded or is invalid, which Init reports as an error during bootstrap.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		c, err := lru.New[string, T](config.App.Cache.Capacity)
		if err != nil {
			panic(errors.Wrap(err, "lru: create cache (run config.Init and cache.Init before requesting caches)"))
		}
		return &cache[T]{c: c}
	})
}

// Set stores the value under key. The backend has no expiration support, so
// only ttl == 0 is accepted.
func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	if ttl > 0 {
		return types.ErrTTLNotSupported
	}
	c.c.Add(key, value)
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
	c.c.Remove(key)
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) bool {
	return c.c.Contains(key)
}
