package redis

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/registry"
	"github.com/hydroan/gst/internal/cache/tracing"
	"github.com/hydroan/gst/types"
	goredis "github.com/redis/go-redis/v9"
)

var (
	_ types.Cache[any] = cache[any]{}

	// handles memoizes the traced handle per value type. The handle is
	// stateless, so one per type serves every caller and the wrapper is not
	// reallocated on each call; session lookups reach this on every
	// authenticated request.
	handles = registry.New()
)

type cache[T any] struct{}

// Cache returns a Redis-backed typed cache handle wrapped with tracing. The
// handle is stateless: the context of each call flows to the Redis client.
//
// Unlike the in-memory backends, which keep one isolated store per value
// type, every redis.Cache shares the application's Redis keyspace: the
// caller-supplied key plus the configured namespace is the storage key.
// Callers own key isolation and build keys through a per-domain key builder
// (such as the iam session key builders). Invalidation paths using the raw
// redis helpers rely on those same builders, which is why the framework must
// not rewrite keys behind the caller's back.
//
// Errors are returned to the caller without logging here; the service and
// controller layers own error reporting.
func Cache[T any]() types.Cache[T] {
	return registry.Load(handles, func() types.Cache[T] {
		return tracing.NewWrapper[T](cache[T]{}, "redis")
	})
}

func (cache[T]) Set(ctx context.Context, key string, data T, ttl time.Duration) error {
	client, err := Client()
	if err != nil {
		return err
	}
	if ttl < 0 {
		return errors.New("negative ttl")
	}
	val, err := json.Marshal(data)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(val) == 0 {
		return errors.New("cannot store empty value in redis")
	}
	// First-hand exit of a stack-less go-redis error; see the error-stack
	// contract in the database package doc. WithStack passes nil through.
	return errors.WithStack(client.Set(ctx, Key(key), val, ttl).Err())
}

func (cache[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T
	client, err := Client()
	if err != nil {
		return zero, err
	}
	data, err := client.Get(ctx, Key(key)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return zero, types.ErrEntryNotFound
		}
		return zero, errors.WithStack(err)
	}
	if len(data) == 0 {
		return zero, types.ErrEntryNotFound
	}
	var result T
	if err = json.Unmarshal(data, &result); err != nil {
		return zero, errors.WithStack(err)
	}
	return result, nil
}

func (cache[T]) Delete(ctx context.Context, key string) error {
	client, err := Client()
	if err != nil {
		return err
	}
	return errors.WithStack(client.Del(ctx, Key(key)).Err())
}

// Exists reports whether key exists. It has no error channel, so a
// disabled client and a Redis failure both answer false.
func (cache[T]) Exists(ctx context.Context, key string) bool {
	client, err := Client()
	if err != nil {
		return false
	}
	res, err := client.Exists(ctx, Key(key)).Result()
	if err != nil {
		return false
	}
	return res > 0
}
