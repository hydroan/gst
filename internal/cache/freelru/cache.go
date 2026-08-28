// Package freelru is a sharded LRU cache that stores live values.
//
// It is the backend that meets the selection criteria without giving
// anything up: writes land and stay, per-entry lifetimes are honored at
// millisecond granularity, the entry bound evicts, and the read and write paths are
// allocation-free and sharded rather than funneled through a worker
// goroutine. Eviction is plain LRU per shard, so it is not scan-resistant —
// the same trade the forwarded default makes.
package freelru

import (
	"context"
	"hash/maphash"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/elastic/go-freelru"
	"github.com/hydroan/gst/internal/cache/capacity"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
)

var (
	store = registry.New()

	// seed randomizes key hashing per process so that externally influenced
	// keys cannot be crafted to collide into one shard.
	seed = maphash.MakeSeed()
)

type cache[T any] struct {
	c *freelru.ShardedLRU[string, T]
}

// Cache returns the process-wide freelru cache of type T, creating it on
// first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		// NewSharded applies the backend's own sizing: one lock domain per
		// core times sixteen, and a quarter more table space than entries so
		// collisions do not evict early. Both matter — hand-picking a small
		// shard count serializes readers on a busy cache.
		c, err := freelru.NewSharded[string, T](capacity.Entries32(), hashKey)
		if err != nil {
			panic(err) // unreachable: the capacity is clamped to a positive range
		}
		return &cache[T]{c: c}
	})
}

func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	// A zero lifetime is the backend's "expired on arrival", so the
	// contract's "never expires" has to go through the plain Add instead.
	if ttl == 0 {
		c.c.Add(key, value)
		return nil
	}
	// The backend keeps expiry at millisecond granularity: a sub-millisecond
	// lifetime truncates to zero and would silently expire on arrival, which
	// the contract forbids.
	if ttl < time.Millisecond {
		return types.ErrTTLNotSupported
	}
	// Round a mid-granularity lifetime up to the next millisecond: the
	// backend truncates, and truncating shortens the promised lifetime.
	if rem := ttl % time.Millisecond; rem != 0 {
		ttl += time.Millisecond - rem
	}
	c.c.AddWithLifetime(key, value, ttl)
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

func hashKey(key string) uint32 {
	// The backend's hash is 32-bit; folding the halves keeps every input bit
	// contributing rather than discarding the high word.
	sum := maphash.String(seed, key)
	return uint32(sum) ^ uint32(sum>>32) //nolint:gosec // deliberate 64-to-32 fold
}
