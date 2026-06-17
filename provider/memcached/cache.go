package memcached

import (
	"context"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/cache/tracing"
	"github.com/hydroan/gst/types"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var _ types.Cache[any] = (*cache[any])(nil)

type cache[T any] struct {
	ctx context.Context
}

func Cache[T any]() types.Cache[T] {
	return tracing.NewWrapper(new(cache[T]), "memcached")
}

func (c *cache[T]) Set(key string, value T, ttl time.Duration) error {
	if !initialized {
		zap.S().Warn("memcached not initialized")
		return errors.New("memcached not initialized")
	}
	val, err := json.Marshal(value)
	if err != nil {
		zap.S().Error(err)
		return err
	}
	expiration := int32(0)
	if ttl > 0 {
		expiration = int32(ttl.Seconds())
	}
	if err := client.Set(&memcache.Item{
		Key:        key,
		Value:      val,
		Expiration: expiration,
	}); err != nil {
		zap.S().Error(err)
		return err
	}
	return nil
}

func (c *cache[T]) Get(key string) (T, error) {
	var zero T
	if !initialized {
		zap.S().Warn("memcached not initialized")
		return zero, errors.New("memcached not initialized")
	}
	item, err := client.Get(key)
	if err != nil {
		if errors.Is(err, memcache.ErrCacheMiss) {
			return zero, types.ErrEntryNotFound
		}
		return zero, err
	}
	var result T
	err = json.Unmarshal(item.Value, &result)
	if err != nil {
		zap.S().Error(err)
		return zero, err
	}
	return result, nil
}
func (c *cache[T]) Peek(key string) (T, error) { return c.Get(key) }
func (c *cache[T]) Delete(key string) error {
	if !initialized {
		zap.S().Warn("memcached not initialized")
		return errors.New("memcached not initialized")
	}
	if err := client.Delete(key); err != nil {
		zap.S().Error(err)
		return err
	}
	return nil
}

func (c *cache[T]) Exists(key string) bool {
	if !initialized {
		zap.S().Warn("memcached not initialized")
		return false
	}
	_, err := client.Get(key)
	if errors.Is(err, memcache.ErrCacheMiss) {
		return false
	}
	return err == nil
}

func (c *cache[T]) Len() int {
	if !initialized {
		zap.S().Warn("memcached not initialized")
		return 0
	}
	// NOTE: memcached don't support.
	return 0
}

func (c *cache[T]) Clear() {
	if !initialized {
		zap.S().Warn("memcached not initialized")
		return
	}
	if err := client.FlushAll(); err != nil {
		zap.S().Error(err)
	}
}

func (c *cache[T]) WithContext(ctx context.Context) types.Cache[T] {
	c.ctx = ctx
	return c
}
