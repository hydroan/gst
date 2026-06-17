package gocache

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/hydroan/gst/cache/tracing"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
	cmap "github.com/orcaman/concurrent-map/v2"
	pkgcache "github.com/patrickmn/go-cache"
)

var (
	cacheMap = cmap.New[any]()
	mu       sync.Mutex
)

func Init() error {
	return nil
}

type cache[T any] struct {
	c   *pkgcache.Cache
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
			c:   pkgcache.New(config.App.Cache.Expiration, config.App.Cache.CleanWindow),
			ctx: context.Background(),
		}, "gocache")
		cacheMap.Set(key, val)
	}
	//nolint:errcheck
	return val.(types.Cache[T])
}

func (c *cache[T]) Set(key string, value T, ttl time.Duration) error {
	c.c.Set(key, value, ttl)
	return nil
}

func (c *cache[T]) Get(key string) (T, error) {
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

func (c *cache[T]) Peek(key string) (T, error) {
	return c.Get(key)
}

func (c *cache[T]) Exists(key string) bool {
	_, exists := c.c.Get(key)
	return exists
}

func (c *cache[T]) Delete(key string) error {
	c.c.Delete(key)
	return nil
}

func (c *cache[T]) Len() int {
	return c.c.ItemCount()
}

func (c *cache[T]) Clear() {
	c.c.Flush()
}

func (c *cache[T]) WithContext(ctx context.Context) types.Cache[T] {
	c.ctx = ctx
	return c
}
