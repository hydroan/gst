package ristretto

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/hydroan/gst/cache/tracing"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
	cmap "github.com/orcaman/concurrent-map/v2"
)

var (
	cacheMap = cmap.New[any]()
	tmp      *ristretto.Cache[string, any] // tmp is a temporary cache used to check the config is correct.
	mu       sync.Mutex
)

func Init() (err error) {
	if tmp, err = ristretto.NewCache(buildConf[any]()); err != nil {
		return err
	}
	tmp.Close()
	return nil
}

type cache[T any] struct {
	c   *ristretto.Cache[string, T]
	ctx context.Context
}

func Cache[T any]() types.Cache[T] {
	typ := reflect.TypeFor[T]()
	key := typ.PkgPath() + "|" + typ.String()
	val, exists := cacheMap.Get(key)
	if exists {
		//nolint:errcheck
		return val.(types.Cache[T])
	}

	mu.Lock()
	defer mu.Unlock()

	val, exists = cacheMap.Get(key)
	if !exists {
		_ristretto, _ := ristretto.NewCache(buildConf[T]())
		val = tracing.NewWrapper(&cache[T]{c: _ristretto, ctx: context.Background()}, "ristretto")
		cacheMap.Set(key, val)
	}
	//nolint:errcheck
	return val.(types.Cache[T])
}

func (c *cache[T]) Set(key string, value T, ttl time.Duration) error {
	if success := c.c.SetWithTTL(key, value, 1, ttl); !success {
		return errors.New("cache rejected the set operation")
	}
	c.c.Wait()
	return nil
}

func (c *cache[T]) Get(key string) (T, error) {
	value, ok := c.c.Get(key)
	if !ok {
		var zero T
		return zero, types.ErrEntryNotFound
	}
	return value, nil
}

func (c *cache[T]) Peek(key string) (T, error) {
	value, ok := c.c.Get(key)
	if !ok {
		var zero T
		return zero, types.ErrEntryNotFound
	}
	return value, nil
}

func (c *cache[T]) Exists(key string) bool {
	_, exists := c.c.Get(key)
	return exists
}

func (c *cache[T]) Delete(key string) error {
	c.c.Del(key)
	return nil
}

func (c *cache[T]) Len() int {
	return -1
}

func (c *cache[T]) Clear() {
	c.c.Clear()
}

func buildConf[T any]() *ristretto.Config[string, T] {
	return &ristretto.Config[string, T]{
		NumCounters: int64(config.App.Cache.Capacity),
		MaxCost:     1 << 30,
		BufferItems: 64,
	}
}

func (c *cache[T]) WithContext(ctx context.Context) types.Cache[T] {
	c.ctx = ctx
	return c
}
