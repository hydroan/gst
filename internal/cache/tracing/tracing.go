// Package tracing wraps cache backends that talk to remote systems with
// OpenTelemetry spans. In-memory backends are not wrapped: a span per
// nanosecond-scale operation costs orders of magnitude more than the
// operation itself.
package tracing

import (
	"context"
	"fmt"
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
	span.SetAttributes(
		attribute.String("cache.operation", "get"),
		attribute.String("cache.key", key),
		attribute.String("cache.type", tw.cacheType),
	)

	value, err := tw.cache.Get(spanCtx, key)
	if err != nil {
		span.SetAttributes(attribute.Bool("cache.hit", false))
		if !errors.Is(err, types.ErrEntryNotFound) {
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed to get cache key: %v", err))
		} else {
			span.SetStatus(codes.Ok, err.Error())
		}
		return value, err
	}

	span.SetAttributes(attribute.Bool("cache.hit", true))
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
	span.SetAttributes(
		attribute.String("cache.operation", "set"),
		attribute.String("cache.key", key),
		attribute.String("cache.ttl", ttl.String()),
		attribute.String("cache.type", tw.cacheType),
	)

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
	span.SetAttributes(
		attribute.String("cache.operation", "delete"),
		attribute.String("cache.key", key),
		attribute.String("cache.type", tw.cacheType),
	)

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
	span.SetAttributes(
		attribute.String("cache.operation", "exists"),
		attribute.String("cache.key", key),
		attribute.String("cache.type", tw.cacheType),
	)

	exists := tw.cache.Exists(spanCtx, key)
	span.SetAttributes(attribute.Bool("cache.exists", exists))
	span.SetStatus(codes.Ok, "Cache key existence checked successfully")
	return exists
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
