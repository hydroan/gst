package dao

import (
	"context"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/dcache"
)

// entryTTL is how long an entry lives. The replicated cache has no shared
// tier, so the ttl is the backstop that reconciles the replicas after a lost
// event: long enough that the scenarios observe propagation rather than
// expiry, short enough that a lost event cannot outlive a test run.
const entryTTL = 10 * time.Minute

// cache opens the replicated cache once and hands the same instance to every
// later call. Opening it takes a kafka producer, a consumer and the wait for
// the consumer group, which is why it happens once, from the component that
// runs before the replica serves.
var cache = sync.OnceValues(dcache.Cache[string])

// OpenCache opens the replicated cache, reporting what kept it from opening.
func OpenCache() error {
	_, err := cache()
	return errors.Wrap(err, "open the replicated cache")
}

// CacheSet writes the entry into this replica's store and publishes it to
// every other replica.
func CacheSet(ctx context.Context, key, value string) error {
	c, err := cache()
	if err != nil {
		return errors.Wrap(err, "open the replicated cache")
	}
	return errors.Wrapf(c.Set(ctx, key, value, entryTTL), "cache the entry at %s", key)
}

// CacheGet reads what this replica's own store holds for the key, without
// asking any other replica: that is what makes a missed event visible.
func CacheGet(ctx context.Context, key string) (string, bool, error) {
	c, err := cache()
	if err != nil {
		return "", false, errors.Wrap(err, "open the replicated cache")
	}
	value, err := c.Get(ctx, key)
	if errors.Is(err, gst.ErrEntryNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.Wrapf(err, "read the entry at %s", key)
	}
	return value, true, nil
}

// CacheDelete removes the entry here and on every other replica.
func CacheDelete(ctx context.Context, key string) error {
	c, err := cache()
	if err != nil {
		return errors.Wrap(err, "open the replicated cache")
	}
	return errors.Wrapf(c.Delete(ctx, key), "remove the entry at %s", key)
}
