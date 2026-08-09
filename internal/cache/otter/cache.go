// Package otter is a W-TinyLFU cache.
//
// It sits between the two failure modes the other backends occupy, because
// write honesty and scan resistance are the same dial. ristretto refuses
// newcomers, so a hot working set survives any sweep and most writes of new
// keys are silently lost. ccache accepts every write, so nothing is lost and
// any sweep evicts the hot set. otter accepts every write and still resists a
// sweep up to about its own capacity: measured on a 200-key hot set in a
// 100k-entry cache, a 100k-key sweep leaves 198 alive where ccache leaves
// none — but a sweep several times the capacity displaces it too.
//
// The dependency is pinned to v2.2.1 on purpose. v2.3.0 ships a mis-named
// test file (issue_test_1.25.go — Go only treats a _test.go suffix as a test
// file) that is compiled as production code under Go 1.25 and later, dragging
// testify into the build graph of every binary that links this backend.
// Upstream issue #185. Once it is fixed, bumping is a one-line change: this
// code compiles unchanged against both versions.
package otter

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/types"
	"github.com/maypok86/otter/v2"
)

const (
	// defaultMaxEntries bounds every per-type cache.
	defaultMaxEntries = 100_000

	// foreverTTL stands in for "never expires": the backend has no such
	// notion and larger values would overflow its deadline arithmetic.
	foreverTTL = 100 * 365 * 24 * time.Hour
)

var store = registry.New()

// entry pairs a cached value with the lifetime its Set asked for. The backend
// configures expiry per cache rather than per write, and hands the calculator
// nothing but the entry, so a per-call ttl has to travel with the value.
type entry[T any] struct {
	value        T
	expiresAfter time.Duration
}

type cache[T any] struct {
	c *otter.Cache[string, entry[T]]
}

// Cache returns the process-wide otter cache of type T, creating it on first
// use.
func Cache[T any]() types.Cache[T] {
	return registry.Load(store, func() types.Cache[T] {
		// Must cannot panic here: the only bound set is a positive maximum
		// size, and the backend only rejects a size paired with a weigher.
		return &cache[T]{c: otter.Must(&otter.Options[string, entry[T]]{
			MaximumSize: maxEntries(),
			// Writing measures the lifetime from the last write and leaves it
			// untouched on reads, so ttl means the same here as everywhere
			// else: time since Set, not time since last use.
			ExpiryCalculator: otter.ExpiryWritingFunc(func(e otter.Entry[string, entry[T]]) time.Duration {
				return e.Value.expiresAfter
			}),
		})}
	})
}

func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	// The backend reads a non-positive lifetime as "leave the deadline
	// alone", so the contract's "never expires" must be mapped to a
	// practically infinite one instead.
	if ttl == 0 {
		ttl = foreverTTL
	}
	c.c.Set(key, entry[T]{value: value, expiresAfter: ttl})
	return nil
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	e, ok := c.c.GetIfPresent(key)
	if !ok {
		var zero T
		return zero, types.ErrEntryNotFound
	}
	return e.value, nil
}

func (c *cache[T]) Delete(_ context.Context, key string) error {
	c.c.Invalidate(key)
	return nil
}

// Exists probes through the read path because the backend keeps its
// containment check unexported. The probe counts as a read, which is
// harmless: statistics are off and write-based expiry ignores reads.
func (c *cache[T]) Exists(_ context.Context, key string) bool {
	_, exists := c.c.GetIfPresent(key)
	return exists
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
