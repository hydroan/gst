package gocache

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	pkgcache "github.com/patrickmn/go-cache"
)

// cleanupInterval is how often the backend sweeps expired entries.
const cleanupInterval = 5 * time.Minute

var store = registry.New()

type cache[T any] struct {
	c *pkgcache.Cache
}

// Cache returns the process-wide go-cache cache of type T, creating it on
// first use. The backend's default expiration is set to its no-expiration
// marker: Set never relies on it because ttl == 0 is mapped explicitly.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		return &cache[T]{c: pkgcache.New(pkgcache.NoExpiration, cleanupInterval)}
	})
}

func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	// The backend treats 0 as "use the default expiration", so the contract's
	// "never expires" must be mapped to its explicit no-expiration marker.
	if ttl == 0 {
		ttl = pkgcache.NoExpiration
	}
	c.c.Set(key, value, ttl)
	return nil
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	var zero T
	val, ok := c.c.Get(key)
	if !ok {
		return zero, types.ErrEntryNotFound
	}
	if val == nil {
		return zero, types.ErrEntryNotFound
	}
	//nolint:errcheck
	return val.(T), nil
}

func (c *cache[T]) Delete(_ context.Context, key string) error {
	c.c.Delete(key)
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) bool {
	_, exists := c.c.Get(key)
	return exists
}
