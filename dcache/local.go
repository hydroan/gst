package dcache

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/hydroan/gst/types"
	cmap "github.com/orcaman/concurrent-map/v2"
	"go.uber.org/zap/zapcore"
)

var (
	// Why cmap v2:
	//  1. sync.Map has no generics, which is awkward in a cache library built around them.
	//  2. cmap v2 performs much better than sync.Map.
	localCacheMap = cmap.New[any]()
	localCacheMu  sync.Mutex
	localMaxItems = 1 << 24
)

var (
	_ CacheMetricsProvider = (*localCache[any])(nil)
	_ types.Cache[any]     = (*localCache[any])(nil)
)

// localCache implements interface Cache use *ristretto as the backend memory localCache.
type localCache[T any] struct {
	c *ristretto.Cache[string, T]
}

// NewLocalCache returns a cache without any distributed capability, use NewDistributedCache when that is needed.
func NewLocalCache[T any]() (types.Cache[T], error) {
	typ := reflect.TypeFor[T]()
	key := typ.PkgPath() + "|" + typ.String()
	val, exists := localCacheMap.Get(key)
	if exists {
		//nolint:errcheck
		return val.(types.Cache[T]), nil
	}

	localCacheMu.Lock()
	defer localCacheMu.Unlock()

	val, exists = localCacheMap.Get(key)
	if !exists {
		c, _ := ristretto.NewCache(buildConf[T]())
		val = &localCache[T]{c: c}
		localCacheMap.Set(key, val)
	}
	//nolint:errcheck
	return val.(types.Cache[T]), nil
}

func (c *localCache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if success := c.c.SetWithTTL(key, value, 1, ttl); !success {
		return errors.New("cache rejected the set operation")
	}
	// Block here until value to be set.
	c.c.Wait()
	return nil
}

func (c *localCache[T]) Get(_ context.Context, key string) (T, error) {
	val, ok := c.c.Get(key)
	if !ok {
		var zero T
		return zero, types.ErrEntryNotFound
	}
	return val, nil
}

// Delete removes the item with the provided key from the cache. Deleting a
// missing key is not an error.
func (c *localCache[T]) Delete(_ context.Context, key string) error {
	c.c.Del(key)
	return nil
}

func (c *localCache[T]) Exists(_ context.Context, key string) bool {
	_, exists := c.c.Get(key)
	return exists
}

func (c *localCache[T]) Metrics() *localMetrics {
	m := c.c.Metrics
	return &localMetrics{
		Misses:       m.Misses(),
		KeysAdded:    m.KeysAdded(),
		KeysUpdated:  m.KeysUpdated(),
		KeysEvicted:  m.KeysEvicted(),
		CostAdded:    m.CostAdded(),
		CostEvicted:  m.CostEvicted(),
		SetsDropped:  m.SetsDropped(),
		SetsRejected: m.SetsRejected(),
		GetsDropped:  m.GetsDropped(),
		GetsKept:     m.GetsKept(),
		Ratio:        m.Ratio(),
	}
}

type localMetrics struct {
	Misses       uint64
	KeysAdded    uint64
	KeysUpdated  uint64
	KeysEvicted  uint64
	CostAdded    uint64
	CostEvicted  uint64
	SetsDropped  uint64
	SetsRejected uint64
	GetsDropped  uint64
	GetsKept     uint64
	Ratio        float64
}

func (m *localMetrics) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m == nil {
		return nil
	}

	enc.AddUint64("misses", m.Misses)
	enc.AddUint64("keys_added", m.KeysAdded)
	enc.AddUint64("keys_updated", m.KeysUpdated)
	enc.AddUint64("keys_evicted", m.KeysEvicted)
	enc.AddUint64("cost_added", m.CostAdded)
	enc.AddUint64("cost_evicted", m.CostEvicted)
	enc.AddUint64("sets_dropped", m.SetsDropped)
	enc.AddUint64("sets_rejected", m.SetsRejected)
	enc.AddUint64("gets_dropped", m.GetsDropped)
	enc.AddUint64("gets_kept", m.GetsKept)
	enc.AddFloat64("ratio", m.Ratio)

	return nil
}

func buildConf[T any]() *ristretto.Config[string, T] {
	return &ristretto.Config[string, T]{
		// NumCounters should be roughly 10 times the maximum number of entries you expect,
		// the value drives the accuracy of the internal bloom filter.
		NumCounters: int64(localMaxItems) * 10,

		// MaxCost is the maximum number of entries to cache,
		// because the cost of every entry is 1.
		MaxCost: int64(localMaxItems),

		// BufferItems controls the size of the write buffer,
		// it is set high here because the entry count is above ten million.
		BufferItems: 256,

		// collect metrics
		Metrics: true,
	}
}
