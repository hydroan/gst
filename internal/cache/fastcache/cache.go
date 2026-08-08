package fastcache

import (
	"context"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

const (
	// defaultCapacityBytes sizes this byte-addressed backend; the backend
	// itself never goes below its 32MB floor.
	defaultCapacityBytes = 64 << 20

	// maxEntrySize is the backend's per-entry ceiling: fastcache silently
	// drops entries whose key plus value exceed its 64KB chunk (minus the
	// length header), so Set enforces the limit with an explicit error
	// instead.
	maxEntrySize = 64*1024 - 4
)

var store = registry.New()

type cache[T any] struct {
	c *fastcache.Cache
}

// Cache returns the process-wide fastcache cache of type T, creating it on
// first use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		return &cache[T]{c: fastcache.New(defaultCapacityBytes)}
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
	if len(key)+len(val) >= maxEntrySize {
		return errors.Newf("entry exceeds the %d byte fastcache limit", maxEntrySize)
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
