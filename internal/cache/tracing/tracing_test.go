package tracing_test

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/cache/tracing"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/internal/types"
	"go.opentelemetry.io/otel/codes"
)

// recordingCache captures the arguments of the last call so tests can assert
// the wrapper forwards them unchanged.
type recordingCache struct {
	lastCtx context.Context
	lastKey string
	lastVal string
	lastTTL time.Duration

	value  string
	getErr error
	exists bool
}

func (c *recordingCache) Get(ctx context.Context, key string) (string, error) {
	c.lastCtx, c.lastKey = ctx, key
	return c.value, c.getErr
}

func (c *recordingCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	c.lastCtx, c.lastKey, c.lastVal, c.lastTTL = ctx, key, value, ttl
	return nil
}

func (c *recordingCache) Delete(ctx context.Context, key string) error {
	c.lastCtx, c.lastKey = ctx, key
	return nil
}

func (c *recordingCache) Exists(ctx context.Context, key string) bool {
	c.lastCtx, c.lastKey = ctx, key
	return c.exists
}

// TestWrapperForwardsWhenTracingDisabled asserts the fast path: with tracing
// disabled every operation reaches the wrapped cache with its arguments and
// results unchanged.
func TestWrapperForwardsWhenTracingDisabled(t *testing.T) {
	inner := &recordingCache{value: "stored", exists: true}
	wrapped := tracing.NewWrapper[string](inner, "sample")
	ctx := context.Background()

	got, err := wrapped.Get(ctx, "key-get")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "stored" {
		t.Fatalf("want %q, got %q", "stored", got)
	}
	if inner.lastCtx != ctx || inner.lastKey != "key-get" {
		t.Fatalf("get args not forwarded: ctx=%v key=%q", inner.lastCtx, inner.lastKey)
	}

	if err := wrapped.Set(ctx, "key-set", "value", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	if inner.lastKey != "key-set" || inner.lastVal != "value" || inner.lastTTL != time.Minute {
		t.Fatalf("set args not forwarded: key=%q val=%q ttl=%v", inner.lastKey, inner.lastVal, inner.lastTTL)
	}

	if err := wrapped.Delete(ctx, "key-delete"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if inner.lastKey != "key-delete" {
		t.Fatalf("delete key not forwarded: %q", inner.lastKey)
	}

	if !wrapped.Exists(ctx, "key-exists") {
		t.Fatal("want true from wrapped Exists")
	}
	if inner.lastKey != "key-exists" {
		t.Fatalf("exists key not forwarded: %q", inner.lastKey)
	}
}

// TestWrapperForwardsErrors asserts errors pass through untouched so callers
// can keep matching sentinels like ErrEntryNotFound.
func TestWrapperForwardsErrors(t *testing.T) {
	inner := &recordingCache{getErr: types.ErrEntryNotFound}
	wrapped := tracing.NewWrapper[string](inner, "sample")

	if _, err := wrapped.Get(context.Background(), "missing"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound, got %v", err)
	}
}

// TestWrapperNormalizesNilContext asserts the contract promise that a nil ctx
// reaches the wrapped cache as a non-nil context.
func TestWrapperNormalizesNilContext(t *testing.T) {
	inner := &recordingCache{}
	wrapped := tracing.NewWrapper[string](inner, "sample")

	var nilCtx context.Context
	if _, err := wrapped.Get(nilCtx, "key"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if inner.lastCtx == nil {
		t.Fatal("want normalized non-nil ctx")
	}
}

// TestWrapperLeavesTheStatusOfACompletedOperationUnset asserts what a cache
// span says about its outcome, the way instrumentation does under the
// OpenTelemetry convention: a hit, a miss, a set, a delete and an existence
// check leave the status unset, the miss being a normal outcome rather than
// a failure, and only an error of the backend marks the span failed.
func TestWrapperLeavesTheStatusOfACompletedOperationUnset(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	inner := &recordingCache{value: "stored", exists: true}
	wrapped := tracing.NewWrapper[string](inner, "sample")
	ctx := context.Background()

	if _, err := wrapped.Get(ctx, "key-hit"); err != nil {
		t.Fatalf("get: %v", err)
	}
	inner.getErr = types.ErrEntryNotFound
	if _, err := wrapped.Get(ctx, "key-miss"); !errors.Is(err, types.ErrEntryNotFound) {
		t.Fatalf("want ErrEntryNotFound, got %v", err)
	}
	if err := wrapped.Set(ctx, "key-set", "value", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := wrapped.Delete(ctx, "key-delete"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !wrapped.Exists(ctx, "key-exists") {
		t.Fatal("want true from wrapped Exists")
	}
	inner.getErr = errors.New("backend down")
	if _, err := wrapped.Get(ctx, "key-failed"); err == nil {
		t.Fatal("want the backend error from Get")
	}

	ended := recorder.Ended()
	if len(ended) != 6 {
		t.Fatalf("want 6 spans, got %d: %v", len(ended), oteltest.EndedNames(recorder))
	}
	for _, span := range ended[:5] {
		if span.Status().Code != codes.Unset {
			t.Errorf("%s: status = %v, want Unset", span.Name(), span.Status().Code)
		}
		if len(span.Events()) != 0 {
			t.Errorf("%s: a completed operation must record no event, got %d", span.Name(), len(span.Events()))
		}
	}
	if failed := ended[5]; failed.Status().Code != codes.Error || len(failed.Events()) == 0 {
		t.Errorf("the backend error must mark the span failed and be recorded: status = %v, events = %d", failed.Status().Code, len(failed.Events()))
	}
}
