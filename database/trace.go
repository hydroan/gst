package database

import (
	"context"
	"reflect"
	"strconv"
	"time"

	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// trace returns a timing function for database operations that provides comprehensive
// performance monitoring, logging, and distributed tracing capabilities.
// The returned function should be called with the operation result to complete tracing and logging.
//
// Parameters:
//   - op: Operation name for logging and tracing identification (Create, List, Update, Delete, etc.)
//   - batch: Optional batch size for batch operations (used for span attributes and logging)
//
// Returns a function that accepts an error and completes the operation tracing and logging.
//
// Features:
//   - Automatic timing measurement from call to completion
//   - OTEL distributed tracing integration with OpenTelemetry spans
//   - Comprehensive span attributes including operation metadata
//   - Error-aware logging and span status management
//   - Batch operation support with size tracking
//   - Dry-run mode status recording
//   - Smart duration formatting for readability
//   - Context propagation to GORM operations
//
// OTEL Tracing Integration:
//   - Creates OpenTelemetry spans with naming pattern: "database.{Model}.{Operation}"
//   - Records detailed span attributes: component, operation, model, table, batch_size, etc.
//   - Propagates span context to GORM operations for complete tracing hierarchy
//   - Automatically handles span lifecycle (creation, attribute setting, completion)
//   - Integrates with existing tracing infrastructure (controller and service layers)
//   - Ensures trace_id is available in database logs through request metadata or
//     the active OTEL span context
//
// Usage Pattern:
//
//	done, _, _ := db.trace("Create", len(models))
//	defer func() { done(err) }()
//
// The closure is load-bearing: a plain `defer done(err)` evaluates err where
// the defer is registered, which is before the operation has run, so every
// span would be finished with a nil error and no failure would ever be
// recorded on it.
//
// Tracing Hierarchy:
//
//	HTTP → Controller → Service → Database → GORM
//
// Note: Must be called after `defer db.reset()` to ensure proper cleanup order.
// Jaeger tracing is automatically enabled when gstotel.IsEnabled() returns true.
func (db *database[M]) trace(op string, batch ...int) (func(error), context.Context, trace.Span) {
	begin := time.Now()
	var _batch int
	if len(batch) > 0 {
		_batch = batch[0]
	}

	ctx := db.ctx
	var span trace.Span
	if gstotel.IsEnabled() && ctx != nil {
		modelName := reflect.TypeOf(*new(M)).Elem().Name()
		spanName := gstotel.FrameworkSpanName("database", modelName, op)
		ctx, span = gstotel.StartSpan(ctx, spanName)
		ctx = requestctx.WithMetadata(ctx, requestctx.FromContext(db.ctx))
		db.ctx = ctx

		// Update GORM database context with new span context
		db.ins = db.ins.WithContext(db.ctx)

		// Performance: submit all attributes of one phase in a single
		// SetAttributes call; every call on a recording span locks the span and
		// re-runs deduplication. When adding attributes, extend the batches in
		// this function instead of adding SetAttributes calls.
		if gstotel.IsSpanRecording(span) {
			attrs := make([]attribute.KeyValue, 0, 7)
			attrs = append(
				attrs,
				attribute.String("component", "database"),
				attribute.String("database.operation", op),
				attribute.String("database.model", modelName),
				attribute.String("database.table", modelName),
				attribute.Bool("database.dry_run", db.dryRun),
			)
			if _batch > 0 {
				attrs = append(attrs, attribute.Int("database.batch_size", _batch))
			}
			span.SetAttributes(attrs...)
		}
	}

	return func(err error) {
		if span != nil {
			defer span.End()
		}

		// Record duration
		duration := time.Since(begin)

		// Update span with results if available; keep this a single batched
		// SetAttributes call (see the performance note above).
		if gstotel.IsSpanRecording(span) {
			attrs := make([]attribute.KeyValue, 0, 2)
			attrs = append(attrs, attribute.Int64("database.duration_ms", duration.Milliseconds()))

			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				gstotel.RecordError(span, err)
				attrs = append(attrs, attribute.Bool("error", true))
			} else {
				span.SetStatus(codes.Ok, "")
			}
			span.SetAttributes(attrs...)
		}

		// Log operation results
		if err != nil {
			logger.Database.WithContext(db.ctx, consts.Phase(op)).Errorz(
				"",
				zap.Error(err),
				zap.String("table", reflect.TypeOf(*new(M)).Elem().Name()),
				zap.String("batch", strconv.Itoa(_batch)),
				util.LogDuration(duration),
				zap.Bool("dry_run", db.dryRun),
			)
		} else {
			logger.Database.WithContext(db.ctx, consts.Phase(op)).Infoz(
				"",
				zap.String("table", reflect.TypeOf(*new(M)).Elem().Name()),
				zap.String("batch", strconv.Itoa(_batch)),
				util.LogDuration(time.Since(begin)),
				zap.Bool("dry_run", db.dryRun),
			)
		}
	}, ctx, span
}

// traceModelHook traces model hook execution with OpenTelemetry spans.
// Creates a span for the hook execution and records timing and error information.
//
// Parameters:
//   - ctx: Database context for span creation
//   - hookName: Name of the hook being executed (CreateBefore, CreateAfter, etc.)
//   - modelName: Name of the model type
//   - fn: Hook function to execute
//
// Returns error from hook execution, with span automatically completed.
//
// Features:
//   - Automatic span creation with naming pattern: "Hook.{HookName} {ModelName}"
//   - Records hook execution timing and success/failure status
//   - Integrates with existing tracing infrastructure
//   - Error recording and span status management
//
// Usage Pattern:
//
//	err := traceModelHook(db.ctx, "CreateBefore", "User", func() error {
//		return obj.CreateBefore()
//	})
func traceModelHook[M types.Model](ctx context.Context, phase consts.Phase, parentSpan trace.Span, fn func(ctx context.Context) error) error {
	hookCtx := context.Background()
	if ctx != nil {
		hookCtx = ctx
	}
	if !gstotel.IsEnabled() || ctx == nil || parentSpan == nil {
		return fn(hookCtx)
	}

	modelName := reflect.TypeOf(*new(M)).Elem().Name()
	// Use a structured gst span name under the database span for hook execution.
	spanName := gstotel.FrameworkSpanName("model", modelName, phase.MethodName())
	parentCtx := trace.ContextWithSpan(hookCtx, parentSpan)
	childCtx, span := gstotel.StartSpan(parentCtx, spanName)
	defer span.End()

	recording := gstotel.IsSpanRecording(span)
	var start time.Time
	if recording {
		// Add hook-specific attributes
		span.SetAttributes(
			attribute.String("component", "model"),
			attribute.String("model.model", modelName),
			attribute.String("model.phase", phase.MethodName()),
		)

		// Record start time
		start = time.Now()
	}

	// Execute hook function
	err := fn(childCtx)

	if recording {
		// Record execution results in a single batched SetAttributes call; every
		// call on a recording span locks the span and re-runs deduplication.
		duration := time.Since(start)
		attrs := make([]attribute.KeyValue, 0, 3)
		attrs = append(
			attrs,
			attribute.Int64("model.duration_ms", duration.Milliseconds()),
			attribute.Bool("model.success", err == nil),
		)

		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			gstotel.RecordError(span, err)
			attrs = append(attrs, attribute.Bool("error", true))
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.SetAttributes(attrs...)
	}

	return err
}
