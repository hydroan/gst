package redis

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"go.uber.org/zap"
)

// Del removes the given keys. Keys that do not exist are not an error: the
// server reports how many it removed, which this drops.
func Del(ctx context.Context, keys ...string) error {
	client, err := Client()
	if err != nil {
		return err
	}
	// First-hand exit of a stack-less go-redis error; see the error-stack
	// contract in the database package doc. WithStack passes nil through.
	return errors.WithStack(client.Del(ctx, namespacedKeys(keys)...).Err())
}

// Expire updates the ttl for an existing key.
func Expire(ctx context.Context, key string, expiration time.Duration) error {
	client, err := Client()
	if err != nil {
		return err
	}
	return errors.WithStack(client.Expire(ctx, Key(key), expiration).Err())
}

// The two values Redis answers TTL with when there is no deadline to report.
// Both are negative, so a caller that compares the result against a positive
// duration has to test for them before reading it as a remaining lifetime.
//
// They are status codes rather than lengths of time: the client hands the
// reply integer back as a Duration without scaling it by the reply's unit, so
// -2 arrives as -2 nanoseconds. Spelling them in seconds would build
// constants no reply can ever equal, and every comparison against them would
// silently report "not this case".
const (
	// TTLKeyNotExists is reported for a key that does not exist.
	TTLKeyNotExists = time.Duration(-2)
	// TTLNoExpiry is reported for a key that exists without a ttl.
	TTLNoExpiry = time.Duration(-1)
)

// TTL reports the remaining ttl of a key.
//
// Redis answers the two "nothing to report" cases with negative durations
// instead of an error, and they are passed through unchanged: TTLKeyNotExists
// for a key that is not there, TTLNoExpiry for a key that is there and never
// expires. A caller comparing the result against a deadline has to rule both
// out first, because either one compares as less than any positive duration.
func TTL(ctx context.Context, key string) (time.Duration, error) {
	client, err := Client()
	if err != nil {
		return 0, err
	}
	ttl, err := client.TTL(ctx, Key(key)).Result()
	return ttl, errors.WithStack(err)
}

// RemovePrefix will scan and delete all redis key that matchs the `prefix`.
// for example: myprefix*
func RemovePrefix(ctx context.Context, prefix string) (err error) {
	client, err := Client()
	if err != nil {
		return err
	}
	iter := client.Scan(ctx, 0, pattern(prefix), 0).Iterator()
	for iter.Next(ctx) {
		err = client.Del(ctx, iter.Val()).Err()
		if err != nil {
			zap.S().Error(err)
			return errors.WithStack(err)
		}
	}
	if err := iter.Err(); err != nil {
		zap.S().Error(err)
		return errors.WithStack(err)
	}
	return nil
}
