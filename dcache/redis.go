package dcache

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
	"github.com/redis/go-redis/v9"
)

// redisCache implements CacheManager interface use Redis as the backend storage.
type redisCache[T any] struct {
	cli redis.UniversalClient // cli is Redis client.

	prefix string
}

// NewRedisCache creates CacheManager implementation that uses Redis as backend.
// It is your responsibility to ensure the redis client is valid.
func NewRedisCache[T any](cli redis.UniversalClient, opts ...RedisCacheOption[T]) (types.Cache[T], error) {
	if cli == nil {
		return nil, errors.New("redis client is nil")
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := cli.Ping(pingCtx).Err(); err != nil {
		return nil, err
	}

	rc := &redisCache[T]{cli: cli}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(rc); err != nil {
			return nil, err
		}
	}
	return rc, nil
}

func (rc *redisCache[T]) Set(ctx context.Context, key string, value T, ttl time.Duration) error {
	ctx = orBackground(ctx)
	// A negative lifetime is not "no lifetime" to the client: -1 is its
	// KEEPTTL sentinel and anything else below zero drops the expiry
	// argument entirely, storing the entry forever.
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	val, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(val) == 0 {
		return errors.New("cannot store empty value in redis")
	}
	return rc.cli.Set(ctx, rc.prefix+key, val, ttl).Err()
}

func (rc *redisCache[T]) Get(ctx context.Context, key string) (T, error) {
	ctx = orBackground(ctx)
	var zero T
	data, err := rc.cli.Get(ctx, rc.prefix+key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return zero, types.ErrEntryNotFound
		}
		return zero, err
	}
	if len(data) == 0 {
		return zero, types.ErrEntryNotFound
	}
	var result T
	if err = json.Unmarshal(data, &result); err != nil {
		return zero, err
	}
	return result, nil
}

func (rc *redisCache[T]) Delete(ctx context.Context, key string) error {
	return rc.cli.Del(orBackground(ctx), rc.prefix+key).Err()
}

func (rc *redisCache[T]) Exists(ctx context.Context, key string) bool {
	res, err := rc.cli.Exists(orBackground(ctx), rc.prefix+key).Result()
	if err != nil {
		return false
	}
	return res > 0
}

// orBackground implements the contract promise that a nil ctx is treated as
// context.Background() before it reaches the Redis client.
func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// RedisCacheOption is used to configure RedisCache.
type RedisCacheOption[T any] func(*redisCache[T]) error

func WithRedisKeyPrefix[T any](prefix string) RedisCacheOption[T] {
	return func(rc *redisCache[T]) error {
		rc.prefix = strings.TrimSpace(prefix)
		return nil
	}
}
