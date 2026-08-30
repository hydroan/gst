package redis_test

import (
	"testing"

	"github.com/hydroan/gst/redis"
)

func TestSortedSetHelpersRoundtrip(t *testing.T) {
	ctx := t.Context()
	key := "redis-test:zset"

	if err := redis.ZAdd(ctx, key, 1, "first"); err != nil {
		t.Fatalf("zadd: %v", err)
	}
	if err := redis.ZAdd(ctx, key, 2, "second", "third"); err != nil {
		t.Fatalf("zadd many: %v", err)
	}

	members, err := redis.ZRange(ctx, key, 0, -1)
	if err != nil {
		t.Fatalf("zrange: %v", err)
	}
	if len(members) != 3 || members[0] != "first" {
		t.Fatalf("want three members in score order, got %v", members)
	}

	members, err = redis.ZRangeByScore(ctx, key, "2", "+inf")
	if err != nil {
		t.Fatalf("zrangebyscore: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("want the two members scored 2, got %v", members)
	}

	if err = redis.ZRem(ctx, key, "first"); err != nil {
		t.Fatalf("zrem: %v", err)
	}
	if err = redis.ZRemRangeByScore(ctx, key, "2", "2"); err != nil {
		t.Fatalf("zremrangebyscore: %v", err)
	}
	if members, err = redis.ZRange(ctx, key, 0, -1); err != nil || len(members) != 0 {
		t.Fatalf("want an empty set, got %v (%v)", members, err)
	}
}
