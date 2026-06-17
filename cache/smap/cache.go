package smap

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hydroan/gst/cache/tracing"
	"github.com/hydroan/gst/types"
	cmap "github.com/orcaman/concurrent-map/v2"
)

var (
	cacheMap = cmap.New[any]()
	mu       sync.Mutex
)

func Init() error {
	return nil
}

type cache[T any] struct {
	m   sync.Map
	n   int64
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
		val = tracing.NewWrapper(&cache[T]{ctx: context.Background()}, "smap")
		cacheMap.Set(key, val)
	}
	//nolint:errcheck
	return val.(types.Cache[T])
}

func (c *cache[T]) Set(key string, value T, ttl time.Duration) error {
	_, loaded := c.m.LoadOrStore(key, value)
	if loaded {
		c.m.Store(key, value)
	} else {
		atomic.AddInt64(&c.n, 1)
	}
	return nil
}

func (c *cache[T]) Get(key string) (T, error) {
	v, ok1 := c.m.Load(key)
	if !ok1 {
		var zero T
		return zero, types.ErrEntryNotFound
	}
	_v, ok2 := v.(T)
	if !ok2 {
		var zero T
		return zero, types.ErrEntryNotFound
	}
	return _v, nil
}

func (c *cache[T]) Peek(key string) (T, error) {
	return c.Get(key)
}

func (c *cache[T]) Delete(key string) error {
	_, exists := c.m.LoadAndDelete(key)
	if exists {
		atomic.AddInt64(&c.n, -1)
	}
	return nil
}

func (c *cache[T]) Exists(key string) bool {
	_, exists := c.m.Load(key)
	return exists
}

func (c *cache[T]) Len() int {
	return int(c.n)
}

func (c *cache[T]) Clear() {
	c.m.Range(func(key, _ any) bool {
		c.m.Delete(key)
		return true
	})
	atomic.StoreInt64(&c.n, 0)
}

func (c *cache[T]) WithContext(ctx context.Context) types.Cache[T] {
	c.ctx = ctx
	return c
}
