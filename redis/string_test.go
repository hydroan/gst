package redis_test

import (
	"context"
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
	// An earlier run in this process may have left the key behind, and the
	// first setnx below only wins on a key that does not exist.
	clearKey(t, "redis-test:setnx")

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

// TestSetXXWritesOnlyAnExistingKey pins what a write that must not recreate a
// removed key depends on: against an absent key it writes nothing and says so,
// and against a present key it overwrites the value and applies the expiration
// it was given.
func TestSetXXWritesOnlyAnExistingKey(t *testing.T) {
	ctx := t.Context()
	// An earlier run in this process may have left the key behind, and the
	// first setxx below must meet an absent key.
	clearKey(t, "redis-test:setxx")

	written, err := redis.SetXX(ctx, "redis-test:setxx", "first", time.Minute)
	if err != nil {
		t.Fatalf("setxx on an absent key: %v", err)
	}
	if written {
		t.Fatal("want setxx to write nothing when the key does not exist")
	}
	if _, err = redis.Get(ctx, "redis-test:setxx"); !errors.Is(err, redis.ErrKeyNotExists) {
		t.Fatalf("want the key to stay absent, got %v", err)
	}

	if err = redis.Set(ctx, "redis-test:setxx", "existing", time.Hour); err != nil {
		t.Fatalf("set: %v", err)
	}
	written, err = redis.SetXX(ctx, "redis-test:setxx", "replaced", time.Minute)
	if err != nil {
		t.Fatalf("setxx on a present key: %v", err)
	}
	if !written {
		t.Fatal("want setxx to overwrite a present key")
	}
	got, err := redis.Get(ctx, "redis-test:setxx")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != "replaced" {
		t.Fatalf("want %q, got %q", "replaced", got)
	}
	// The key was set to live an hour; a remaining lifetime within the minute
	// passed to setxx shows the new expiration replaced the old one.
	ttl, err := redis.TTL(ctx, "redis-test:setxx")
	if err != nil {
		t.Fatalf("ttl: %v", err)
	}
	if ttl <= 0 || ttl > time.Minute {
		t.Fatalf("want the expiration passed to setxx, got ttl %v", ttl)
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
	// The counts below start from an absent key.
	clearKey(t, "redis-test:counter")

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

// TestIncrFixedWindowBoundsTheCountToItsFirstWindow pins what a counter that
// must reset on schedule depends on: the count starts at one with its window
// set, later increments neither reset the count nor move the window, and a
// count found without a ttl is given one instead of counting forever.
func TestIncrFixedWindowBoundsTheCountToItsFirstWindow(t *testing.T) {
	ctx := t.Context()

	t.Run("first_increment_opens_the_window", func(t *testing.T) {
		key := "redis-test:fixed-window:first"
		clearKey(t, key)

		count, err := redis.IncrFixedWindow(ctx, key, time.Hour)
		if err != nil {
			t.Fatalf("incr: %v", err)
		}
		if count != 1 {
			t.Fatalf("want 1 from the first increment, got %d", count)
		}
		ttl, err := redis.TTL(ctx, key)
		if err != nil {
			t.Fatalf("ttl: %v", err)
		}
		if ttl <= 0 || ttl > time.Hour {
			t.Fatalf("want a ttl within the window, got %v", ttl)
		}
	})

	t.Run("later_increments_keep_the_first_window", func(t *testing.T) {
		key := "redis-test:fixed-window:later"
		clearKey(t, key)

		if _, err := redis.IncrFixedWindow(ctx, key, time.Hour); err != nil {
			t.Fatalf("first incr: %v", err)
		}
		// A day-long window on the second increment would push the ttl past
		// the hour if the window were set again.
		count, err := redis.IncrFixedWindow(ctx, key, 24*time.Hour)
		if err != nil {
			t.Fatalf("second incr: %v", err)
		}
		if count != 2 {
			t.Fatalf("want 2 from the second increment, got %d", count)
		}
		ttl, err := redis.TTL(ctx, key)
		if err != nil {
			t.Fatalf("ttl: %v", err)
		}
		if ttl <= 0 || ttl > time.Hour {
			t.Fatalf("want the first window kept, got ttl %v", ttl)
		}
	})

	t.Run("a_count_without_ttl_is_given_the_window", func(t *testing.T) {
		key := "redis-test:fixed-window:no-ttl"
		clearKey(t, key)

		// The shape a two-command increment leaves when its process stops
		// before setting the ttl.
		if _, err := redis.Incr(ctx, key); err != nil {
			t.Fatalf("incr without ttl: %v", err)
		}
		if ttl, err := redis.TTL(ctx, key); err != nil || ttl != redis.TTLNoExpiry {
			t.Fatalf("precondition: want a count without ttl, got %v (%v)", ttl, err)
		}

		count, err := redis.IncrFixedWindow(ctx, key, time.Minute)
		if err != nil {
			t.Fatalf("incr: %v", err)
		}
		if count != 2 {
			t.Fatalf("want 2, got %d", count)
		}
		ttl, err := redis.TTL(ctx, key)
		if err != nil {
			t.Fatalf("ttl: %v", err)
		}
		if ttl <= 0 || ttl > time.Minute {
			t.Fatalf("want the window set on the count, got ttl %v", ttl)
		}
	})

	t.Run("rejects_a_window_redis_cannot_keep_exactly", func(t *testing.T) {
		key := "redis-test:fixed-window:invalid"
		clearKey(t, key)

		for _, window := range []time.Duration{0, -time.Second, 500 * time.Microsecond, 1500 * time.Microsecond} {
			if _, err := redis.IncrFixedWindow(ctx, key, window); err == nil {
				t.Fatalf("want an error for window %v", window)
			}
		}
		if ttl, err := redis.TTL(ctx, key); err != nil || ttl != redis.TTLKeyNotExists {
			t.Fatalf("a rejected window must leave the key untouched, got ttl %v (%v)", ttl, err)
		}
	})

	// A key holding another type makes the server refuse the increment; that
	// refusal must reach the caller carrying the stack of its first-hand exit.
	t.Run("reports_backend_errors_with_a_stack", func(t *testing.T) {
		key := "redis-test:fixed-window:wrong-type"
		clearKey(t, key)

		if err := redis.ZAdd(ctx, key, 1, "member"); err != nil {
			t.Fatalf("zadd: %v", err)
		}
		_, err := redis.IncrFixedWindow(ctx, key, time.Minute)
		if err == nil {
			t.Fatal("want the backend error for a key holding a sorted set")
		}
		if errors.GetReportableStackTrace(err) == nil {
			t.Fatalf("want a run-time stack on the backend error, got none: %v", err)
		}
	})

	t.Run("reports_redis_not_enabled", func(t *testing.T) {
		if err := redis.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		t.Cleanup(func() {
			if err := redis.Init(); err != nil {
				t.Fatalf("reconnect: %v", err)
			}
		})

		if _, err := redis.IncrFixedWindow(ctx, "redis-test:fixed-window:closed", time.Minute); !errors.Is(err, redis.ErrRedisIsDisabled) {
			t.Fatalf("want ErrRedisIsDisabled, got %v", err)
		}
	})
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
	// The backend error must carry the run-time stack embedded at its
	// first-hand exit, per the error-stack contract in the database package
	// doc, so the error_stack log field can locate the caller.
	if errors.GetReportableStackTrace(err) == nil {
		t.Fatalf("want a run-time stack on the backend error, got none: %v", err)
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
	} else if errors.GetReportableStackTrace(err) == nil {
		t.Fatalf("want a run-time stack on the decode error, got none: %v", err)
	}
}

// clearKey deletes key now and again when the test ends, so a test that
// depends on the key being absent holds in a repeated run of the process.
func clearKey(t *testing.T, key string) {
	t.Helper()
	if err := redis.Del(t.Context(), key); err != nil {
		t.Fatalf("del %s: %v", key, err)
	}
	t.Cleanup(func() { _ = redis.Del(context.Background(), key) })
}
