package redis

import (
	"context"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	goredis "github.com/redis/go-redis/v9"
)

// Set set any data into redis with specific key.
// If the data type is custom type or structure, you must implement the interface encoding.BinaryMarshaler.
func Set(ctx context.Context, key string, data any, expiration ...time.Duration) error {
	client, err := Client()
	if err != nil {
		return err
	}
	ttl := config.App.Redis.Expiration
	if len(expiration) > 0 {
		ttl = expiration[0]
	}
	// First-hand exit of a stack-less go-redis error: embed the run-time
	// stack so the error_stack log field can locate any caller without
	// call-site logging (see the error-stack contract in the database
	// package doc). WithStack passes nil through.
	return errors.WithStack(client.Set(ctx, Key(key), data, ttl).Err())
}

// Get will get raw cache([]byte) from redis.
func Get(ctx context.Context, key string) (cache []byte, err error) {
	client, err := Client()
	if err != nil {
		return nil, err
	}
	cache, err = client.Get(ctx, Key(key)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrKeyNotExists
		}
		return nil, errors.WithStack(err)
	}
	return cache, nil
}

// SetNX sets key to value with expiration only when the key does not already exist.
func SetNX(ctx context.Context, key, value string, expiration time.Duration) (bool, error) {
	client, err := Client()
	if err != nil {
		return false, err
	}
	ok, err := client.SetNX(ctx, Key(key), value, expiration).Result()
	return ok, errors.WithStack(err)
}

// Incr increments the integer at key by one and returns the new value,
// creating the key at zero first when it does not exist.
//
// The read and the write are one Redis operation, which is what makes it usable
// as a counter under concurrency: a caller that instead read, added, and wrote
// back would lose increments to every interleaving of two requests.
func Incr(ctx context.Context, key string) (int64, error) {
	client, err := Client()
	if err != nil {
		return 0, err
	}
	count, err := client.Incr(ctx, Key(key)).Result()
	return count, errors.WithStack(err)
}

// GetInt get cache from redis and decode into integer.
func GetInt(ctx context.Context, key string) (int64, error) {
	client, err := Client()
	if err != nil {
		return 0, err
	}
	cache, err := client.Get(ctx, Key(key)).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return 0, ErrKeyNotExists
		}
		return 0, errors.WithStack(err)
	}
	val, err := strconv.Atoi(cache)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return int64(val), nil
}
