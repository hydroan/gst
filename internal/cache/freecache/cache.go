package freecache

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/coocood/freecache"
	"github.com/hydroan/gst/internal/cache/codec"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
)

// defaultCapacityBytes sizes this byte-addressed backend. The backend caps a
// single entry at 1/1024 of the cache size, so 64MB also fixes the per-entry
// ceiling at 64KB.
const defaultCapacityBytes = 64 << 20

var store = registry.New()

type cache[T any] struct {
	c *freecache.Cache
}

// Cache returns the process-wide freecache cache of type T, creating it on
// first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		return &cache[T]{c: freecache.NewCache(defaultCapacityBytes)}
	})
}

func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	val, err := codec.Marshal(value)
	if err != nil {
		return err
	}
	// The backend has second granularity: a sub-second lifetime would have
	// to be stretched to a full second — silently mis-honoring the request,
	// which the contract forbids — so it is rejected instead.
	if ttl > 0 && ttl < time.Second {
		return types.ErrTTLNotSupported
	}
	// Round a mid-granularity lifetime up to the next second: truncating
	// would shorten the promised lifetime.
	seconds := 0
	if ttl > 0 {
		seconds = int(ttl / time.Second)
		if ttl%time.Second != 0 {
			seconds++
		}
	}
	return c.c.Set([]byte(key), val, seconds)
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	var zero T
	val, err := c.c.Get([]byte(key))
	if err != nil {
		// The backend only fails a Get for a missing or expired entry.
		return zero, types.ErrEntryNotFound
	}
	var result T
	if err := codec.Unmarshal(val, &result); err != nil {
		return zero, err
	}
	return result, nil
}

func (c *cache[T]) Delete(_ context.Context, key string) error {
	c.c.Del([]byte(key))
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) bool {
	_, err := c.c.Get([]byte(key))
	return err == nil
}
