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

// SetXX sets key to value with expiration only when the key already exists,
// and reports whether it did.
//
// It is the write for a key that must not come back once removed. Checking for
// the key and then setting it are two commands a delete can fall between, and
// the set would then recreate what was just removed; here the check and the
// write are one Redis command.
func SetXX(ctx context.Context, key, value string, expiration time.Duration) (bool, error) {
	client, err := Client()
	if err != nil {
		return false, err
	}
	ok, err := client.SetXX(ctx, Key(key), value, expiration).Result()
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

// IncrFixedWindow increments the integer at key by one and returns the new
// value, bounding the count to a window that opens with its first increment.
//
// The window is set only while the key has no ttl — on the increment that
// creates the key, or on one that finds a count left without a ttl — and
// later increments never extend it, so the count always resets a fixed time
// after it began. Extending the window on every increment would let anyone who
// keeps incrementing hold the count up indefinitely, and a count without a ttl
// would never reset at all.
//
// The increment and the ttl travel as one MULTI/EXEC transaction on a single
// key. No process can stop between the two and leave a count that never
// expires, and because the transaction names one key it stays on one slot in
// cluster mode; widening it to more keys would break that.
//
// window must be a positive whole number of milliseconds, the precision Redis
// keeps ttls in; any other value is rejected rather than rounded.
func IncrFixedWindow(ctx context.Context, key string, window time.Duration) (int64, error) {
	if window < time.Millisecond || window%time.Millisecond != 0 {
		return 0, errors.Newf("fixed window must be a positive whole number of milliseconds, got %s", window)
	}
	client, err := Client()
	if err != nil {
		return 0, err
	}
	namespaced := Key(key)
	var count *goredis.IntCmd
	if _, err = client.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
		count = pipe.Incr(ctx, namespaced)
		pipe.Do(ctx, "pexpire", namespaced, window.Milliseconds(), "nx")
		return nil
	}); err != nil {
		// First-hand exit of a stack-less go-redis error; see the error-stack
		// contract in the database package doc.
		return 0, errors.WithStack(err)
	}
	return count.Val(), nil
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
