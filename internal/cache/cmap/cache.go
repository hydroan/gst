package cmap

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	cmap "github.com/orcaman/concurrent-map/v2"
)

var store = registry.New()

func Init() error { return nil }

type cache[T any] struct {
	c cmap.ConcurrentMap[string, T]
}

// Cache returns the process-wide concurrent-map cache of type T, creating it
// on first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		return &cache[T]{c: cmap.New[T]()}
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
	c.c.Set(key, value)
	return nil
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	value, exists := c.c.Get(key)
	if !exists {
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
	return c.c.Has(key)
}
