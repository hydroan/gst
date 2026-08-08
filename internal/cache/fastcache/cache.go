package fastcache

import (
	"context"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

var store = registry.New()

func Init() error { return nil }

type cache[T any] struct {
	c *fastcache.Cache
}

// Cache returns the process-wide fastcache cache of type T, creating it on
// first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		return &cache[T]{c: fastcache.New(config.App.Cache.Capacity)}
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
	val, err := util.Marshal(value)
	if err != nil {
		return err
	}
	c.c.Set([]byte(key), val)
	return nil
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	var zero T
	value, ok := c.c.HasGet(nil, []byte(key))
	if !ok {
		return zero, types.ErrEntryNotFound
	}
	var result T
	if err := util.Unmarshal(value, &result); err != nil {
		return zero, err
	}
	return result, nil
}

func (c *cache[T]) Delete(_ context.Context, key string) error {
	c.c.Del([]byte(key))
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) bool {
	return c.c.Has([]byte(key))
}
