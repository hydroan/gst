package ccache

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/hydroan/gst/cache/tracing"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
	"github.com/karlseguin/ccache/v3"
	cmap "github.com/orcaman/concurrent-map/v2"
)

var (
	cacheMap = cmap.New[any]()
	mu       sync.Mutex
)

func Init() (err error) {
	return nil
}

type cache[T any] struct {
	c   *ccache.Cache[T]
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
			c:   ccache.New(ccache.Configure[T]().MaxSize(int64(config.App.Cache.Capacity))),
			ctx: context.Background(),
		}, "ccache")
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
	val := c.c.Get(key)
	if val == nil {
		return zero, types.ErrEntryNotFound
	}
	if val.Expired() {
		return zero, types.ErrEntryNotFound
	}
	return val.Value(), nil
}

func (c *cache[T]) Peek(key string) (T, error) {
	return c.Get(key)
}

func (c *cache[T]) Exists(key string) bool {
	val := c.c.Get(key)
	if val == nil {
		return false
	}
	if val.Expired() {
		return false
	}
	return true
}

func (c *cache[T]) Delete(key string) error {
	c.c.Delete(key)
	return nil
}

func (c *cache[T]) Len() int {
	return c.c.ItemCount()
}

func (c *cache[T]) Clear() {
	c.c.Clear()
}

func (c *cache[T]) WithContext(ctx context.Context) types.Cache[T] {
	c.ctx = ctx
	return c
}
