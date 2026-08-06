package database

import (
	"context"
	"reflect"
	"time"

	"github.com/cockroachdb/errors"
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

// Phases of database-layer operations that have no controller counterpart in
// consts. Values follow the consts.Phase snake_case convention so the log
// phase field stays uniform across every log source; span names derive their
// UpperCamelCase form via Phase.MethodName.
const (
	phaseUpsert               consts.Phase = "upsert"
	phaseCount                consts.Phase = "count"
	phaseFirst                consts.Phase = "first"
	phaseLast                 consts.Phase = "last"
	phaseTake                 consts.Phase = "take"
	phaseUpdateByID           consts.Phase = "update_by_id"
	phaseAggregate            consts.Phase = "aggregate"
	phaseAggregateOne         consts.Phase = "aggregate_one"
	phaseAggregateCountGroups consts.Phase = "aggregate_count_groups"
	phaseCleanup              consts.Phase = "cleanup"
	phaseHealth               consts.Phase = "health"
	phaseTransaction          consts.Phase = "transaction"
	phaseWithQuery            consts.Phase = "with_query"
)

// trace returns a timing function for database operations that provides comprehensive
// performance monitoring, logging, and distributed tracing capabilities.
// The returned function should be called with the operation result to complete tracing and logging.
//
// Parameters:
//   - phase: Operation phase for logging and tracing identification
//     (consts.PHASE_CREATE, consts.PHASE_LIST, phaseUpsert, etc.)
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
//	done, _ := db.trace(consts.PHASE_CREATE, len(models))
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
func (db *database[M]) trace(phase consts.Phase, batch ...int) (func(error), trace.Span) {
	begin := time.Now()
	modelName := reflect.TypeOf(*new(M)).Elem().Name()
	var _batch int
	if len(batch) > 0 {
		_batch = batch[0]
	}

	ctx := db.ctx
	var span trace.Span
	if gstotel.IsEnabled() && ctx != nil {
		spanName := gstotel.FrameworkSpanName("database", modelName, phase.MethodName())
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
			attrs := make([]attribute.KeyValue, 0, 6)
			attrs = append(
				attrs,
				attribute.String("component", "database"),
				attribute.String("database.operation", phase.MethodName()),
				attribute.String("database.model", modelName),
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

		// Log operation results. Success and record-not-found stay at debug
		// level: both are normal outcomes whose timing the SQL log and the
		// operation span already cover, so they only matter when tracing an
		// operation end to end. Real failures log at error level. Constant
		// markers (batch_size, dry_run) appear only when meaningful.
		fields := make([]zap.Field, 0, 5)
		fields = append(fields, zap.String("model", modelName))
		if _batch > 0 {
			fields = append(fields, zap.Int("batch_size", _batch))
		}
		fields = append(fields, util.LogDuration(duration))
		if db.dryRun {
			fields = append(fields, zap.Bool("dry_run", true))
		}
		switch {
		case err == nil:
			logger.Database.WithContext(db.ctx, phase).Debugz("database operation completed", fields...)
		case errors.Is(err, ErrRecordNotFound):
			logger.Database.WithContext(db.ctx, phase).Debugz("database operation completed", append(fields, zap.Bool("record_not_found", true))...)
		default:
			logger.Database.WithContext(db.ctx, phase).Errorz("database operation failed", append(fields, zap.Error(err))...)
		}
	}, span
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
