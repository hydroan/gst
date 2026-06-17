// Package lrue is a expirable lru cache.
package lrue

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/hydroan/gst/cache/tracing"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
	cmap "github.com/orcaman/concurrent-map/v2"
)

var (
	cacheMap = cmap.New[any]()
	mu       sync.Mutex
)

func Init() error { return nil }

type cache[T any] struct {
	c   *expirable.LRU[string, T]
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
		val = tracing.NewWrapper(&cache[T]{
			c:   expirable.NewLRU[string, T](config.App.Cache.Capacity, nil, config.App.Cache.Expiration),
			ctx: context.Background(),
		}, "lrue")
		cacheMap.Set(key, val)
	}
	//nolint:errcheck
	return val.(types.Cache[T])
}

func (c *cache[T]) Set(key string, value T, ttl time.Duration) error {
	c.c.Add(key, value)
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

func (c *cache[T]) Delete(key string) error {
	c.c.Remove(key)
	return nil
}

func (c *cache[T]) Exists(key string) bool {
	return c.c.Contains(key)
}

func (c *cache[T]) Len() int {
	return c.c.Len()
}

func (c *cache[T]) Clear() {
	c.c.Purge()
}

func (c *cache[T]) WithContext(ctx context.Context) types.Cache[T] {
	c.ctx = ctx
	return c
}
