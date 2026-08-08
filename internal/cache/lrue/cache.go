// Package lrue is an expirable lru cache with one global expiration window
// shared by every entry.
package lrue

import (
	"context"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
)

const (
	// defaultMaxEntries bounds every per-type cache.
	defaultMaxEntries = 100_000
	// defaultExpiration is the single global window every entry shares.
	defaultExpiration = 10 * time.Minute
)

var store = registry.New()

type cache[T any] struct {
	c *expirable.LRU[string, T]
}

// Cache returns the process-wide expirable lru cache of type T, creating it
// on first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		return &cache[T]{
			c: expirable.NewLRU[string, T](maxEntries(), nil, defaultExpiration),
		}
	})
}

// Set always returns ErrTTLNotSupported: entries in this backend expire on
// the global window fixed at construction, so neither ttl == 0 (never expire)
// nor a per-entry ttl can be honored. The backend stays wired for the day the
// contract grows an opt-in for global expiration.
func (c *cache[T]) Set(context.Context, string, T, time.Duration) error {
	return types.ErrTTLNotSupported
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

// maxEntries returns the configured per-type entry bound, falling back to
// the built-in default when it is unset or the configuration is not loaded
// yet.
func maxEntries() int {
	if v := config.App.Cache.MaxEntries; v > 0 {
		return v
	}
	return defaultMaxEntries
}
