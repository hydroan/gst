package gocache

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	pkgcache "github.com/patrickmn/go-cache"
)

var store = registry.New()

func Init() error { return nil }

type cache[T any] struct {
	c *pkgcache.Cache
}

// Cache returns the process-wide go-cache cache of type T, creating it on
// first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		return &cache[T]{c: pkgcache.New(config.App.Cache.Expiration, config.App.Cache.CleanWindow)}
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
