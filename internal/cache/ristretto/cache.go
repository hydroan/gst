package ristretto

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
)

var store = registry.New()

func Init() error {
	tmp, err := ristretto.NewCache(buildConf[any]())
	if err != nil {
		return err
	}
	tmp.Close()
	return nil
}

type cache[T any] struct {
	c *ristretto.Cache[string, T]
}

// Cache returns the process-wide ristretto cache of type T, creating it on
// first use. Creation failures panic: they mean the cache configuration was
// never loaded or is invalid, which Init reports as an error during bootstrap.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		c, err := ristretto.NewCache(buildConf[T]())
		if err != nil {
			panic(errors.Wrap(err, "ristretto: create cache (run config.Init and cache.Init before requesting caches)"))
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

// buildConf sizes the cache from the configured capacity: MaxCost bounds the
// entry count because every entry costs 1, and NumCounters is ten times the
// expected entry count as the admission policy requires.
func buildConf[T any]() *ristretto.Config[string, T] {
	capacity := int64(config.App.Cache.Capacity)
	return &ristretto.Config[string, T]{
		NumCounters: capacity * 10,
		MaxCost:     capacity,
		BufferItems: 64,
	}
}
