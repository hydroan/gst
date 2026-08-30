package redis_test

import (
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/redis"
)

// TestSetNXIsExclusive pins the semantics every caller building a lock or a
// one-time-use marker depends on: the first writer of a key is told it won,
// and every later one is told it lost while the key lives.
func TestSetNXIsExclusive(t *testing.T) {
	ctx := t.Context()

	acquired, err := redis.SetNX(ctx, "redis-test:setnx", "first", time.Minute)
	if err != nil {
		t.Fatalf("first setnx: %v", err)
	}
	if !acquired {
		t.Fatal("want the first setnx to acquire the key")
	}

	acquired, err = redis.SetNX(ctx, "redis-test:setnx", "second", time.Minute)
	if err != nil {
		t.Fatalf("second setnx: %v", err)
	}
	if acquired {
		t.Fatal("want the second setnx to lose the key")
	}
}

func TestStringHelpersRoundtrip(t *testing.T) {
	ctx := t.Context()

	if err := redis.Set(ctx, "redis-test:string", "value", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := redis.Get(ctx, "redis-test:string")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != "value" {
		t.Fatalf("want %q, got %q", "value", got)
	}

	if err = redis.Del(ctx, "redis-test:string"); err != nil {
		t.Fatalf("del: %v", err)
	}
	if _, err = redis.Get(ctx, "redis-test:string"); !errors.Is(err, redis.ErrKeyNotExists) {
		t.Fatalf("want ErrKeyNotExists after del, got %v", err)
	}
}

func TestCounterHelpersRoundtrip(t *testing.T) {
	ctx := t.Context()

	count, err := redis.Incr(ctx, "redis-test:counter")
	if err != nil {
		t.Fatalf("first incr: %v", err)
	}
	if count != 1 {
		t.Fatalf("want 1 from the first incr, got %d", count)
	}
	if count, err = redis.Incr(ctx, "redis-test:counter"); err != nil || count != 2 {
		t.Fatalf("want 2 from the second incr, got %d (%v)", count, err)
	}

	stored, err := redis.GetInt(ctx, "redis-test:counter")
	if err != nil {
		t.Fatalf("getint: %v", err)
	}
	if stored != 2 {
		t.Fatalf("want 2, got %d", stored)
	}
}

// TestTTLReportsLifetime covers the three answers TTL carries: a remaining
// lifetime, the marker for a key without a deadline, and the marker for a key
// that is not there. The two markers arrive instead of an error, so the
// exported constants have to equal what the client actually returns.
func TestTTLReportsLifetime(t *testing.T) {
	ctx := t.Context()

	if err := redis.Set(ctx, "redis-test:ttl", "value", time.Hour); err != nil {
		t.Fatalf("set: %v", err)
	}
	ttl, err := redis.TTL(ctx, "redis-test:ttl")
	if err != nil {
		t.Fatalf("ttl: %v", err)
	}
	if ttl <= 0 || ttl > time.Hour {
		t.Fatalf("want a remaining lifetime within the hour, got %v", ttl)
	}

	if err = redis.Expire(ctx, "redis-test:ttl", 2*time.Hour); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if ttl, err = redis.TTL(ctx, "redis-test:ttl"); err != nil || ttl <= time.Hour {
		t.Fatalf("want the extended lifetime, got %v (%v)", ttl, err)
	}

	if ttl, err = redis.TTL(ctx, "redis-test:ttl-missing"); err != nil || ttl != redis.TTLKeyNotExists {
		t.Fatalf("want TTLKeyNotExists, got %v (%v)", ttl, err)
	}

	if err = redis.Set(ctx, "redis-test:ttl-forever", "value", 0); err != nil {
		t.Fatalf("set without a ttl: %v", err)
	}
	if ttl, err = redis.TTL(ctx, "redis-test:ttl-forever"); err != nil || ttl != redis.TTLNoExpiry {
		t.Fatalf("want TTLNoExpiry, got %v (%v)", ttl, err)
	}
}

// TestReadHelpersReportBackendErrors asserts that a failure the server reports
// reaches the caller. Reading a sorted set as a string is the reproducible
// case: the server answers WRONGTYPE, which is neither a missing key nor a
// value the caller can use, and must not be flattened into a successful read.
func TestReadHelpersReportBackendErrors(t *testing.T) {
	ctx := t.Context()
	key := "redis-test:wrong-type"
	if err := redis.ZAdd(ctx, key, 1, "member"); err != nil {
		t.Fatalf("zadd: %v", err)
	}

	got, err := redis.Get(ctx, key)
	if err == nil {
		t.Fatalf("want the backend error from Get, got a successful read of %q", got)
	}
	if errors.Is(err, redis.ErrKeyNotExists) {
		t.Fatalf("want the backend error, got ErrKeyNotExists: %v", err)
	}

	count, err := redis.GetInt(ctx, key)
	if err == nil {
		t.Fatalf("want the backend error from GetInt, got %d", count)
	}
	if errors.Is(err, redis.ErrKeyNotExists) {
		t.Fatalf("want the backend error, got ErrKeyNotExists: %v", err)
	}
	if strings.Contains(err.Error(), "strconv") {
		t.Fatalf("want the backend error, got a decode error of the empty value: %v", err)
	}
}

// TestGetIntReportsUndecodableValue keeps the decode failure distinct from a
// backend failure: a key holding a non-numeric string is a real read whose
// value the caller cannot use.
func TestGetIntReportsUndecodableValue(t *testing.T) {
	ctx := t.Context()
	if err := redis.Set(ctx, "redis-test:not-a-number", "value", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}

	if _, err := redis.GetInt(ctx, "redis-test:not-a-number"); err == nil {
		t.Fatal("want a decode error for a non-numeric value")
	} else if !strings.Contains(err.Error(), "strconv") {
		t.Fatalf("want the decode error, got %v", err)
	}
}

func TestModelHelpersRoundtrip(t *testing.T) {
	ctx := t.Context()
	want := &Group{Name: "sample", Desc: "one", MemberCount: 3}

	if err := redis.SetM(ctx, "redis-test:model", want, time.Minute); err != nil {
		t.Fatalf("setm: %v", err)
	}
	got, err := redis.GetM[*Group](ctx, "redis-test:model")
	if err != nil {
		t.Fatalf("getm: %v", err)
	}
	if got.Name != want.Name || got.MemberCount != want.MemberCount {
		t.Fatalf("want %+v, got %+v", want, got)
	}

	if err = redis.SetML(ctx, "redis-test:model-list", []*Group{want}, time.Minute); err != nil {
		t.Fatalf("setml: %v", err)
	}
	list, err := redis.GetML[*Group](ctx, "redis-test:model-list")
	if err != nil {
		t.Fatalf("getml: %v", err)
	}
	if len(list) != 1 || list[0].Name != want.Name {
		t.Fatalf("want one %q back, got %+v", want.Name, list)
	}
}

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

// TestRemovePrefixDeletesMatchingKeysOnly asserts the scan-and-delete sweep
// clears the keys under the prefix and leaves the rest of the keyspace alone.
func TestRemovePrefixDeletesMatchingKeysOnly(t *testing.T) {
	ctx := t.Context()

	for _, key := range []string{"redis-test:sweep:one", "redis-test:sweep:two"} {
		if err := redis.Set(ctx, key, "value", time.Minute); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	if err := redis.Set(ctx, "redis-test:kept", "value", time.Minute); err != nil {
		t.Fatalf("set the key outside the prefix: %v", err)
	}

	if err := redis.RemovePrefix(ctx, "redis-test:sweep"); err != nil {
		t.Fatalf("removeprefix: %v", err)
	}

	for _, key := range []string{"redis-test:sweep:one", "redis-test:sweep:two"} {
		if _, err := redis.Get(ctx, key); !errors.Is(err, redis.ErrKeyNotExists) {
			t.Fatalf("want %s swept, got %v", key, err)
		}
	}
	if _, err := redis.Get(ctx, "redis-test:kept"); err != nil {
		t.Fatalf("want the key outside the prefix kept: %v", err)
	}
}
