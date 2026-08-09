package dcache

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap/zapcore"
)

// defaultMaxEntries bounds every per-type local cache when the cache
// configuration does not override it. Instances are created lazily per type,
// so only used types pay the admission-counter cost of roughly 4MB per
// instance at this capacity.
const defaultMaxEntries = 100_000

var (
	// One instance per value type, keyed by the type itself; see the note on
	// distributedCacheMap for why this is a sync.Map.
	localCacheMap sync.Map
	localCacheMu  sync.Mutex
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
	key := reflect.TypeFor[T]()
	if val, ok := localCacheMap.Load(key); ok {
		//nolint:errcheck
		return val.(types.Cache[T]), nil
	}

	localCacheMu.Lock()
	defer localCacheMu.Unlock()

	if val, ok := localCacheMap.Load(key); ok {
		//nolint:errcheck
		return val.(types.Cache[T]), nil
	}
	c, err := ristretto.NewCache(buildConf[T]())
	if err != nil {
		// Storing a cache built from a failed constructor would answer every
		// Get with a miss and every Set with a rejection, which reads as a
		// working but permanently empty cache.
		return nil, err
	}
	val := &localCache[T]{c: c}
	localCacheMap.Store(key, val)
	return val, nil
}

func (c *localCache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if success := c.c.SetWithTTL(key, value, 1, ttl); !success {
		return errors.New("cache rejected the set operation")
	}
	// Block until the buffered set is applied so a Set followed by a Get on
	// the same key observes the value (read-your-write).
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

// buildConf sizes the local tier following the backend's own guidance:
// MaxCost bounds the entry count because every entry costs 1, NumCounters is
// ten times the expected entries to keep the admission sketch accurate, and
// BufferItems is the recommended write-buffer size. Metrics stay enabled
// because the distributed cache monitor reports them.
func buildConf[T any]() *ristretto.Config[string, T] {
	entries := int64(maxEntries())
	return &ristretto.Config[string, T]{
		NumCounters: entries * 10,
		MaxCost:     entries,
		BufferItems: 64,
		// Every entry costs exactly 1 so MaxCost bounds the entry count; the
		// backend would otherwise add its per-item bookkeeping bytes to each
		// cost and silently shrink the capacity about 57-fold.
		IgnoreInternalCost: true,
		Metrics:            true,
	}
}

// maxEntries returns the configured per-type entry bound, falling back to
// the built-in default when it is unset or the configuration is not loaded
// yet.
func maxEntries() int {
	if v := config.App.Cache.MaxEntries; v > 0 {
		return v
	}
	return defaultMaxEntries
}
