package ristretto

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
)

// defaultMaxEntries bounds every per-type cache. The framework ships a fixed
// default instead of a configuration knob: per-type instances are created
// lazily and the admission counters cost memory per instance, so the default
// stays small; raise it here when a real workload needs more.
const defaultMaxEntries = 10_000

var store = registry.New()

type cache[T any] struct {
	c *ristretto.Cache[string, T]
}

// Cache returns the process-wide ristretto cache of type T, creating it on
// first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		c, err := ristretto.NewCache(buildConf[T]())
		if err != nil {
			panic(err) // unreachable: the configuration is built from positive constants
		}
		return &cache[T]{c: c}
	})
}

func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	if success := c.c.SetWithTTL(key, value, 1, ttl); !success {
		return errors.New("cache rejected the set operation")
	}
	// Block until the buffered set is applied so a Set followed by a Get on
	// the same key observes the value (read-your-write).
	c.c.Wait()
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
	c.c.Del(key)
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) bool {
	_, exists := c.c.Get(key)
	return exists
}

// buildConf sizes the cache following the backend's own guidance: MaxCost
// bounds the entry count because every entry costs 1, NumCounters is ten
// times the expected entries to keep the admission sketch accurate, and
// BufferItems is the recommended write-buffer size.
func buildConf[T any]() *ristretto.Config[string, T] {
	return &ristretto.Config[string, T]{
		NumCounters: defaultMaxEntries * 10,
		MaxCost:     defaultMaxEntries,
		BufferItems: 64,
	}
}
