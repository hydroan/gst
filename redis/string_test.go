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
