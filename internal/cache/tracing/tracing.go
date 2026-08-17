// Package tracing wraps cache backends that talk to remote systems with
// OpenTelemetry spans. In-memory backends are not wrapped: a span per
// nanosecond-scale operation costs orders of magnitude more than the
// operation itself.
//
// Spans never carry a cache key verbatim. Callers routinely build keys from
// bearer credentials — session ids that are the login cookie's own value,
// MFA challenge ids, password-reset tokens — so exporting the key would
// stream live credentials to whatever collects traces, where they are
// readable by anyone with access to it and replayable until they expire.
// Each span instead carries the key's namespace, which says which cache
// domain was touched, and a truncated digest, which lets the same key be
// correlated across spans without being reversible. Everything a cache span
// is normally read for survives that substitution.
package tracing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Wrapper decorates a Cache implementation with distributed tracing. It is
// stateless: the caller context of each operation parents its span, and when
// tracing is disabled every operation forwards to the wrapped cache directly.
type Wrapper[T any] struct {
	cache     types.Cache[T]
	cacheType string
}

var _ types.Cache[any] = (*Wrapper[any])(nil)

// NewWrapper creates a new tracing wrapper for the given cache.
func NewWrapper[T any](cache types.Cache[T], cacheType string) *Wrapper[T] {
	return &Wrapper[T]{cache: cache, cacheType: cacheType}
}

// Get retrieves a value by key with tracing.
func (tw *Wrapper[T]) Get(ctx context.Context, key string) (T, error) {
	ctx = orBackground(ctx)
	if !gstotel.IsEnabled() {
		return tw.cache.Get(ctx, key)
	}
	spanCtx, span := tw.startSpan(ctx, "get")
	defer span.End()
	// Attributes are built only for a span that records them: the key digest is
	// a SHA-256 hash, so assembling them unconditionally would hash a key on
	// every cache operation of every sampled-out request. They are submitted in
	// one call once the outcome is known, because each call on a recording span
	// locks it and re-runs attribute deduplication.
	recording := gstotel.IsSpanRecording(span)

	value, err := tw.cache.Get(spanCtx, key)
	if recording {
		span.SetAttributes(append(tw.attributes("get", key), attribute.Bool("cache.hit", err == nil))...)
	}
	if err != nil {
		if !errors.Is(err, types.ErrEntryNotFound) {
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed to get cache key: %v", err))
		} else {
			span.SetStatus(codes.Ok, err.Error())
		}
		return value, err
	}

	span.SetStatus(codes.Ok, "Cache key retrieved successfully")
	return value, nil
}

// Set stores a key-value pair with tracing.
func (tw *Wrapper[T]) Set(ctx context.Context, key string, value T, ttl time.Duration) error {
	ctx = orBackground(ctx)
	if !gstotel.IsEnabled() {
		return tw.cache.Set(ctx, key, value, ttl)
	}
	spanCtx, span := tw.startSpan(ctx, "set")
	defer span.End()
	if gstotel.IsSpanRecording(span) {
		span.SetAttributes(append(tw.attributes("set", key), attribute.String("cache.ttl", ttl.String()))...)
	}

	if err := tw.cache.Set(spanCtx, key, value, ttl); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("Failed to set cache key: %v", err))
		return err
	}

	span.SetStatus(codes.Ok, "Cache key set successfully")
	return nil
}

// Delete removes a key from the cache with tracing.
func (tw *Wrapper[T]) Delete(ctx context.Context, key string) error {
	ctx = orBackground(ctx)
	if !gstotel.IsEnabled() {
		return tw.cache.Delete(ctx, key)
	}
	spanCtx, span := tw.startSpan(ctx, "delete")
	defer span.End()
	if gstotel.IsSpanRecording(span) {
		span.SetAttributes(tw.attributes("delete", key)...)
	}

	if err := tw.cache.Delete(spanCtx, key); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("Failed to delete cache key: %v", err))
		return err
	}

	span.SetStatus(codes.Ok, "Cache key deleted successfully")
	return nil
}

// Exists checks if a key exists in the cache with tracing.
func (tw *Wrapper[T]) Exists(ctx context.Context, key string) bool {
	ctx = orBackground(ctx)
	if !gstotel.IsEnabled() {
		return tw.cache.Exists(ctx, key)
	}
	spanCtx, span := tw.startSpan(ctx, "exists")
	defer span.End()
	recording := gstotel.IsSpanRecording(span)

	exists := tw.cache.Exists(spanCtx, key)
	if recording {
		span.SetAttributes(append(tw.attributes("exists", key), attribute.Bool("cache.exists", exists))...)
	}
	span.SetStatus(codes.Ok, "Cache key existence checked successfully")
	return exists
}

// attributes builds the span attributes for one cache operation. Every span
// attribute the wrapper sets goes through here, so the rule that a key is
// never recorded verbatim holds in one place and can be tested in one place.
func (tw *Wrapper[T]) attributes(operation, key string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("cache.operation", operation),
		attribute.String("cache.key.namespace", keyNamespace(key)),
		attribute.String("cache.key.digest", keyDigest(key)),
		attribute.String("cache.type", tw.cacheType),
	}
}

// keyNamespace returns the key up to its last separator, naming the cache
// domain without the identifier that follows it. Keys with no separator have
// no namespace to report rather than being reported whole.
func keyNamespace(key string) string {
	i := strings.LastIndex(key, ":")
	if i < 0 {
		return ""
	}
	return key[:i]
}

// keyDigest returns a truncated SHA-256 of the key, enough to correlate the
// same key across spans and not enough to recover it.
func keyDigest(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

// orBackground implements the contract promise that a nil ctx is treated as
// context.Background(); normalizing here keeps every wrapped backend safe.
func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// startSpan creates a new span for the given cache operation as a child of
// ctx. The caller owns the returned span and must end it after the cache
// operation finishes.
func (tw *Wrapper[T]) startSpan(ctx context.Context, operation string) (context.Context, trace.Span) {
	tracer := gstotel.GetTracer()
	operationName := gstotel.OperationSpanName("cache", operation)
	spanCtx, span := tracer.Start(ctx, operationName) //nolint:spancheck // Caller receives and ends the returned span.
	return spanCtx, span                              //nolint:spancheck // Caller receives and ends the returned span.
}
