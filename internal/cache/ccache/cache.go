package ccache

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	"github.com/karlseguin/ccache/v3"
)

const (
	// defaultMaxEntries bounds every per-type cache.
	defaultMaxEntries = 10_000

	// foreverTTL stands in for "never expires": the backend has no such
	// notion and larger values would overflow its deadline arithmetic.
	foreverTTL = 100 * 365 * 24 * time.Hour
)

var store = registry.New()

type cache[T any] struct {
	c *ccache.Cache[T]
}

// Cache returns the process-wide ccache cache of type T, creating it on first
// use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		return &cache[T]{c: ccache.New(ccache.Configure[T]().MaxSize(defaultMaxEntries))}
	})
}

func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	// The backend expires an entry ttl after Set, so the contract's "never
	// expires" must be mapped to a practically infinite lifetime.
	if ttl == 0 {
		ttl = foreverTTL
	}
	c.c.Set(key, value, ttl)
	return nil
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	var zero T
	val := c.c.Get(key)
	if val == nil {
		return zero, types.ErrEntryNotFound
	}
	if val.Expired() {
		return zero, types.ErrEntryNotFound
	}
	return val.Value(), nil
}

func (c *cache[T]) Delete(_ context.Context, key string) error {
	c.c.Delete(key)
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) bool {
	val := c.c.Get(key)
	if val == nil {
		return false
	}
	return !val.Expired()
}
