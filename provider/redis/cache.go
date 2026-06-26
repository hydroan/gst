package redis

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/cache/tracing"
	"github.com/hydroan/gst/types"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var _ types.Cache[any] = (*cache[any])(nil)

type cache[T any] struct {
	ctx context.Context
}

// Cache returns a new Redis-backed typed cache handle.
//
// Each call creates a fresh handle, so callers may bind a context once and
// reuse the returned value for multiple related Redis cache operations in the
// same flow:
//
//	cache := redis.Cache[T]().WithContext(ctx)
//
// This guarantee is specific to this Redis provider and should not be assumed
// for other cache implementations. The generic types.Cache contract does not
// require WithContext to return an independent handle.
func Cache[T any]() types.Cache[T] {
	return tracing.NewWrapper(&cache[T]{ctx: context.Background()}, "redis")
}

func (c *cache[T]) Set(key string, data T, ttl time.Duration) error {
	if !initialized {
		zap.S().Warn("redis not initialized")
		return errors.New("redis not initialized")
	}
	val, err := json.Marshal(data)
	if err != nil {
		zap.S().Error(err)
		return err
	}
	if len(val) == 0 {
		return errors.New("cannot store empty value in redis")
	}
	if err = cli.Set(c.ctx, redisKey(key), val, ttl).Err(); err != nil {
		zap.S().Error(err)
		return err
	}
	return nil
}

func (c *cache[T]) Get(key string) (T, error) {
	var zero T
	if !initialized {
		zap.S().Warn("redis not initialized")
		return zero, errors.New("redis not initialized")
	}
	data, err := cli.Get(c.ctx, redisKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return zero, types.ErrEntryNotFound
		}
		zap.S().Error(err)
		return zero, err
	}
	if len(data) == 0 {
		return zero, types.ErrEntryNotFound
	}
	var result T
	if err = json.Unmarshal(data, &result); err != nil {
		zap.S().Error(err)
		return zero, err
	}
	return result, nil
}

func (c *cache[T]) Peek(key string) (T, error) {
	if !initialized {
		zap.S().Warn("redis not initialized")
		return *new(T), errors.New("redis not initialized")
	}
	return c.Get(key)
}

func (c *cache[T]) Delete(key string) error {
	if !initialized {
		zap.S().Warn("redis not initialized")
		return errors.New("redis not initialized")
	}
	if err := cli.Del(c.ctx, redisKey(key)).Err(); err != nil {
		zap.S().Error(err)
		return err
	}
	return nil
}

func (c *cache[T]) Exists(key string) bool {
	if !initialized {
		zap.S().Warn("redis not initialized")
		return false
	}
	res, err := cli.Exists(c.ctx, redisKey(key)).Result()
	if err != nil {
		return false
	}
	return res > 0
}

func (c *cache[T]) Len() int {
	if !initialized {
		zap.S().Warn("redis not initialized")
		return 0
	}
	// In Redis Cluster, this only counts the selected node.
	count, err := cli.DBSize(c.ctx).Result()
	if err != nil {
		zap.S().Error(err)
		return 0
	}
	return int(count)
}

func (c *cache[T]) Clear() {
	if !initialized {
		zap.S().Warn("redis not initialized")
		return
	}
	if err := RemovePrefix(c.ctx, ""); err != nil {
		zap.S().Error(err)
	}
}

// WithContext returns a new handle bound to ctx without mutating the receiver.
// This is a stronger guarantee than the generic types.Cache interface requires.
func (c *cache[T]) WithContext(ctx context.Context) types.Cache[T] { return &cache[T]{ctx: ctx} }
