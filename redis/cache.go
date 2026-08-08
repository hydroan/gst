package redis

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/tracing"
	"github.com/hydroan/gst/types"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var _ types.Cache[any] = cache[any]{}

type cache[T any] struct{}

// Cache returns a Redis-backed typed cache handle wrapped with tracing. The
// handle is stateless: the context of each call flows to the Redis client.
func Cache[T any]() types.Cache[T] {
	return tracing.NewWrapper[T](cache[T]{}, "redis")
}

func (cache[T]) Set(ctx context.Context, key string, data T, ttl time.Duration) error {
	if cli == nil {
		return errors.New("redis not initialized")
	}
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	val, err := json.Marshal(data)
	if err != nil {
		zap.S().Error(err)
		return err
	}
	if len(val) == 0 {
		return errors.New("cannot store empty value in redis")
	}
	if err = cli.Set(ctx, redisKey(key), val, ttl).Err(); err != nil {
		zap.S().Error(err)
		return err
	}
	return nil
}

func (cache[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T
	if cli == nil {
		return zero, errors.New("redis not initialized")
	}
	data, err := cli.Get(ctx, redisKey(key)).Bytes()
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

func (cache[T]) Delete(ctx context.Context, key string) error {
	if cli == nil {
		return errors.New("redis not initialized")
	}
	if err := cli.Del(ctx, redisKey(key)).Err(); err != nil {
		zap.S().Error(err)
		return err
	}
	return nil
}

func (cache[T]) Exists(ctx context.Context, key string) bool {
	if cli == nil {
		zap.S().Warn("redis not initialized")
		return false
	}
	res, err := cli.Exists(ctx, redisKey(key)).Result()
	if err != nil {
		return false
	}
	return res > 0
}
