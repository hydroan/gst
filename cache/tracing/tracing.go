package tracing

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Wrapper wraps a Cache implementation with distributed tracing capabilities
type Wrapper[T any] struct {
	cache     types.Cache[T]
	ctx       context.Context
	cacheType string
}

// NewWrapper creates a new tracing wrapper for the given cache
func NewWrapper[T any](cache types.Cache[T], cacheType string) *Wrapper[T] {
	return &Wrapper[T]{
		cache:     cache,
		ctx:       context.Background(),
		cacheType: cacheType,
	}
}

func (tw *Wrapper[T]) WithContext(ctx context.Context) types.Cache[T] {
	if ctx != nil {
		tw.ctx = ctx
	}
	return tw
}

// Set stores a key-value pair with tracing
func (tw *Wrapper[T]) Set(key string, value T, ttl time.Duration) error {
	spanCtx, span := tw.startSpan("Cache.Set")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("cache.operation", "set"),
		attribute.String("cache.key", key),
		attribute.String("cache.ttl", ttl.String()),
		attribute.String("cache.type", tw.cacheType),
	)

	// Record start time for duration measurement
	start := time.Now()

	// Call the underlying cache implementation with span context
	err := tw.cache.WithContext(spanCtx).Set(key, value, ttl)

	// Record operation duration
	duration := time.Since(start)
	span.SetAttributes(
		attribute.String("cache.duration", duration.String()),
	)

	if err != nil {
		if !errors.Is(err, types.ErrEntryNotFound) {
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed to set cache key: %v", err))
		} else {
			span.SetStatus(codes.Ok, err.Error())
		}
		return err
	}

	span.SetStatus(codes.Ok, "Cache key set successfully")
	return nil
}

// Get retrieves a value by key with tracing
func (tw *Wrapper[T]) Get(key string) (T, error) {
	spanCtx, span := tw.startSpan("Cache.Get")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("cache.operation", "get"),
		attribute.String("cache.key", key),
		attribute.String("cache.type", tw.cacheType),
	)

	// Record start time for duration measurement
	start := time.Now()

	// Call the underlying cache implementation with span context
	value, err := tw.cache.WithContext(spanCtx).Get(key)

	// Record operation duration
	duration := time.Since(start)
	span.SetAttributes(
		attribute.String("cache.duration", duration.String()),
	)

	if err != nil {
		span.SetAttributes(
			attribute.Bool("cache.hit", false),
		)
		if !errors.Is(err, types.ErrEntryNotFound) {
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed to get cache key: %v", err))
		} else {
			span.SetStatus(codes.Ok, err.Error())
		}
		return value, err
	}

	span.SetAttributes(
		attribute.Bool("cache.hit", true),
	)
	span.SetStatus(codes.Ok, "Cache key retrieved successfully")
	return value, nil
}

// Peek retrieves a value by key without affecting its position with tracing
func (tw *Wrapper[T]) Peek(key string) (T, error) {
	spanCtx, span := tw.startSpan("Cache.Peek")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("cache.operation", "peek"),
		attribute.String("cache.key", key),
		attribute.String("cache.type", tw.cacheType),
	)

	// Record start time for duration measurement
	start := time.Now()

	// Call the underlying cache implementation with span context
	value, err := tw.cache.WithContext(spanCtx).Peek(key)

	// Record operation duration
	duration := time.Since(start)
	span.SetAttributes(
		attribute.String("cache.duration", duration.String()),
	)

	if err != nil {
		span.SetAttributes(
			attribute.Bool("cache.hit", false),
		)
		if !errors.Is(err, types.ErrEntryNotFound) {
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed to peek cache key: %v", err))
		} else {
			span.SetStatus(codes.Ok, err.Error())
		}
		return value, err
	}

	span.SetAttributes(
		attribute.Bool("cache.hit", true),
	)
	span.SetStatus(codes.Ok, "Cache key peeked successfully")
	return value, nil
}

// Delete removes a key from the cache with tracing
func (tw *Wrapper[T]) Delete(key string) error {
	spanCtx, span := tw.startSpan("Cache.Delete")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("cache.operation", "delete"),
		attribute.String("cache.key", key),
		attribute.String("cache.type", tw.cacheType),
	)

	// Record start time for duration measurement
	start := time.Now()

	// Call the underlying cache implementation with span context
	err := tw.cache.WithContext(spanCtx).Delete(key)

	// Record operation duration
	duration := time.Since(start)
	span.SetAttributes(
		attribute.String("cache.duration", duration.String()),
	)

	if err != nil {
		if !errors.Is(err, types.ErrEntryNotFound) {
			span.RecordError(err)
			span.SetStatus(codes.Error, fmt.Sprintf("Failed to delete cache key: %v", err))
		} else {
			span.SetStatus(codes.Ok, err.Error())
		}
		return err
	}

	span.SetStatus(codes.Ok, "Cache key deleted successfully")
	return nil
}

// Exists checks if a key exists in the cache with tracing
func (tw *Wrapper[T]) Exists(key string) bool {
	spanCtx, span := tw.startSpan("Cache.Exists")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("cache.operation", "exists"),
		attribute.String("cache.key", key),
		attribute.String("cache.type", tw.cacheType),
	)

	// Record start time for duration measurement
	start := time.Now()

	// Call the underlying cache implementation with span context
	exists := tw.cache.WithContext(spanCtx).Exists(key)

	// Record operation duration
	duration := time.Since(start)
	span.SetAttributes(
		attribute.String("cache.duration", duration.String()),
		attribute.Bool("cache.exists", exists),
	)

	span.SetStatus(codes.Ok, "Cache key existence checked successfully")
	return exists
}

// Len returns the number of items in the cache with tracing
func (tw *Wrapper[T]) Len() int {
	spanCtx, span := tw.startSpan("Cache.Len")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("cache.operation", "len"),
		attribute.String("cache.type", tw.cacheType),
	)

	// Record start time for duration measurement
	start := time.Now()

	// Call the underlying cache implementation with span context
	length := tw.cache.WithContext(spanCtx).Len()

	// Record operation duration
	duration := time.Since(start)
	span.SetAttributes(
		attribute.String("cache.duration", duration.String()),
		attribute.Int("cache.length", length),
	)

	span.SetStatus(codes.Ok, "Cache length retrieved successfully")
	return length
}

// Clear removes all items from the cache with tracing
func (tw *Wrapper[T]) Clear() {
	spanCtx, span := tw.startSpan("Cache.Clear")
	defer span.End()

	// Add span attributes
	span.SetAttributes(
		attribute.String("cache.operation", "clear"),
		attribute.String("cache.type", tw.cacheType),
	)

	// Record start time for duration measurement
	start := time.Now()

	// Call the underlying cache implementation with span context
	tw.cache.WithContext(spanCtx).Clear()

	// Record operation duration
	duration := time.Since(start)
	span.SetAttributes(
		attribute.String("cache.duration", duration.String()),
	)

	span.SetStatus(codes.Ok, "Cache cleared successfully")
}

// startSpan creates a new span for the given operation. The caller owns the
// returned span and must end it after the cache operation finishes.
func (tw *Wrapper[T]) startSpan(operationName string) (context.Context, trace.Span) {
	tracer := gstotel.GetTracer()
	ctx, span := tracer.Start(tw.ctx, operationName) //nolint:spancheck // Caller receives and ends the returned span.
	return ctx, span                                 //nolint:spancheck // Caller receives and ends the returned span.
}
