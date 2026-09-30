package redis_test

import (
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	gstredis "github.com/hydroan/gst/redis"
)

// TestTTLReportsLifetime covers the three answers TTL carries: a remaining
// lifetime, the marker for a key without a deadline, and the marker for a key
// that is not there. The two markers arrive instead of an error, so the
// exported constants have to equal what the client actually returns.
func TestTTLReportsLifetime(t *testing.T) {
	ctx := t.Context()

	if err := gstredis.Set(ctx, "redis-test:ttl", "value", time.Hour); err != nil {
		t.Fatalf("set: %v", err)
	}
	ttl, err := gstredis.TTL(ctx, "redis-test:ttl")
	if err != nil {
		t.Fatalf("ttl: %v", err)
	}
	if ttl <= 0 || ttl > time.Hour {
		t.Fatalf("want a remaining lifetime within the hour, got %v", ttl)
	}

	if err = gstredis.Expire(ctx, "redis-test:ttl", 2*time.Hour); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if ttl, err = gstredis.TTL(ctx, "redis-test:ttl"); err != nil || ttl <= time.Hour {
		t.Fatalf("want the extended lifetime, got %v (%v)", ttl, err)
	}

	if ttl, err = gstredis.TTL(ctx, "redis-test:ttl-missing"); err != nil || ttl != gstredis.TTLKeyNotExists {
		t.Fatalf("want TTLKeyNotExists, got %v (%v)", ttl, err)
	}

	if err = gstredis.Set(ctx, "redis-test:ttl-forever", "value", 0); err != nil {
		t.Fatalf("set without a ttl: %v", err)
	}
	if ttl, err = gstredis.TTL(ctx, "redis-test:ttl-forever"); err != nil || ttl != gstredis.TTLNoExpiry {
		t.Fatalf("want TTLNoExpiry, got %v (%v)", ttl, err)
	}
}

// TestRemovePrefixDeletesMatchingKeysOnly asserts the scan-and-delete sweep
// clears the keys under the prefix and leaves the rest of the keyspace alone.
func TestRemovePrefixDeletesMatchingKeysOnly(t *testing.T) {
	ctx := t.Context()

	for _, key := range []string{"redis-test:sweep:one", "redis-test:sweep:two"} {
		if err := gstredis.Set(ctx, key, "value", time.Minute); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	if err := gstredis.Set(ctx, "redis-test:kept", "value", time.Minute); err != nil {
		t.Fatalf("set the key outside the prefix: %v", err)
	}

	if err := gstredis.RemovePrefix(ctx, "redis-test:sweep"); err != nil {
		t.Fatalf("removeprefix: %v", err)
	}

	for _, key := range []string{"redis-test:sweep:one", "redis-test:sweep:two"} {
		if _, err := gstredis.Get(ctx, key); !errors.Is(err, gstredis.ErrKeyNotExists) {
			t.Fatalf("want %s swept, got %v", key, err)
		}
	}
	if _, err := gstredis.Get(ctx, "redis-test:kept"); err != nil {
		t.Fatalf("want the key outside the prefix kept: %v", err)
	}
}
