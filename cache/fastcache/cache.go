package fastcache

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/hydroan/gst/cache/tracing"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
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
	c   *fastcache.Cache
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
		val = tracing.NewWrapper(&cache[T]{c: fastcache.New(config.App.Cache.Capacity), ctx: context.Background()}, "fastcache")
		cacheMap.Set(key, val)
	}
	//nolint:errcheck
	return val.(types.Cache[T])
}

func (c *cache[T]) Set(key string, value T, ttl time.Duration) error {
	val, err := util.Marshal(value)
	if err != nil {
		return err
	}
	c.c.Set([]byte(key), val)
	return nil
}

func (c *cache[T]) Get(key string) (T, error) {
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

func (c *cache[T]) Peek(key string) (T, error) {
	return c.Get(key)
}

func (c *cache[T]) Delete(key string) error {
	c.c.Del([]byte(key))
	return nil
}

func (c *cache[T]) Exists(key string) bool {
	return c.c.Has([]byte(key))
}

func (c *cache[T]) Len() int {
	return 0
}

func (c *cache[T]) Clear() {
	c.c.Reset()
}

func (c *cache[T]) WithContext(ctx context.Context) types.Cache[T] {
	c.ctx = ctx
	return c
}
