package ccache

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	"github.com/karlseguin/ccache/v3"
)

const (
	// defaultMaxEntries bounds every per-type cache.
	defaultMaxEntries = 100_000

	// foreverTTL stands in for "never expires": the backend has no such
	// notion and larger values would overflow its deadline arithmetic.
	foreverTTL = 100 * 365 * 24 * time.Hour
)

var store = registry.New()

// box wraps the cached value so the backend never sees the caller's type
// directly. The backend's newItem asserts each stored value against its Sized
// interface (Size() int64) and, on a match, charges the reported size against
// MaxSize instead of counting the entry as one — silently turning the entry
// bound into a cost budget for any T that happens to carry that method.
// Boxing makes the assertion structurally impossible; it costs one struct
// field and no extra allocation.
type box[T any] struct{ v T }

// cache is a per-type instance. It is intentionally immortal: the backend's
// Stop is never called, because its worker goroutine is the only drain for
// the promote and delete channels while set writes to them unconditionally.
// After a Stop every write would wedge forever once the 1024-slot buffers
// fill. Instances are per-type and live for the whole process, so there is
// nothing to stop.
type cache[T any] struct {
	c *ccache.Cache[box[T]]
}

// Cache returns the process-wide ccache cache of type T, creating it on first
// use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		return &cache[T]{c: ccache.New(ccache.Configure[box[T]]().MaxSize(int64(maxEntries())))}
	})
}

func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	// The backend expires an entry ttl after Set, so the contract's "never
	// expires" must be mapped to a practically infinite lifetime.
	if ttl == 0 {
		ttl = foreverTTL
	}
	c.c.Set(key, box[T]{v: value}, ttl)
	return nil
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	val := c.c.Get(key)
	if val == nil || val.Expired() {
		var zero T
		return zero, types.ErrEntryNotFound
	}
	return val.Value().v, nil
}

func (c *cache[T]) Delete(_ context.Context, key string) error {
	c.c.Delete(key)
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) bool {
	val := c.c.Get(key)
	if val == nil {
		return false
	}
	return !val.Expired()
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
