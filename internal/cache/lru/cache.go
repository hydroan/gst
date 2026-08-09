package lru

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hydroan/gst/internal/cache/capacity"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
)

var store = registry.New()

type cache[T any] struct {
	c *lru.Cache[string, T]
}

// Cache returns the process-wide lru cache of type T, creating it on first
// use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		c, err := lru.New[string, T](capacity.Entries())
		if err != nil {
			panic(err) // unreachable: the default capacity is a positive constant
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
