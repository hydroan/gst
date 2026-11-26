// Package smap adapts sync.Map to gst.Cache: a map of live values with no
// expiration and no entry bound. An entry stays until it is deleted, and a
// lifetime is refused with gst.ErrTTLNotSupported.
package smap

import (
	"context"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/internal/gst"
)

var store = registry.New()

type cache[T any] struct {
	m sync.Map
}

// Cache returns the process-wide sync.Map cache of type T, creating it on
// first use.
func Cache[T any]() gst.Cache[T] {
	return registry.Load(store, func() gst.Cache[T] {
		return new(cache[T])
	})
}

// Set stores the value under key. The backend has no expiration support, so
// only ttl == 0 is accepted.
func (c *cache[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	if ttl > 0 {
		return gst.ErrTTLNotSupported
	}
	c.m.Store(key, value)
	return nil
}

func (c *cache[T]) Get(_ context.Context, key string) (T, error) {
	var zero T
	v, ok := c.m.Load(key)
	if !ok {
		return zero, gst.ErrEntryNotFound
	}
	value, ok := v.(T)
	if !ok {
		return zero, gst.ErrEntryNotFound
	}
	return value, nil
}

func (c *cache[T]) Delete(_ context.Context, key string) error {
	c.m.Delete(key)
	return nil
}

func (c *cache[T]) Exists(_ context.Context, key string) (bool, error) {
	_, exists := c.m.Load(key)
	return exists, nil
}
