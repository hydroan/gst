package database

import (
	"context"
	"reflect"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
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
	phaseUpsert        consts.Phase = "upsert"
	phaseCount         consts.Phase = "count"
	phaseFirst         consts.Phase = "first"
	phaseLast          consts.Phase = "last"
	phaseTake          consts.Phase = "take"
	phaseUpdateByID    consts.Phase = "update_by_id"
	phaseSelect        consts.Phase = "select"
	phaseSelectOne     consts.Phase = "select_one"
	phaseSelectCount   consts.Phase = "select_count"
	phaseUnionAll      consts.Phase = "union_all"
	phaseUnionAllCount consts.Phase = "union_all_count"
	phaseCleanup       consts.Phase = "cleanup"
	phaseHealth        consts.Phase = "health"
	phaseWithQuery     consts.Phase = "with_query"
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
//   - Error-aware logging and span status management, with record-not-found
//     treated as a normal outcome by both
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
	return db.traceAs(reflect.TypeOf(*new(M)).Elem().Name(), phase, batch...)
}

// The attribute and field batches below are sized to their worst case once,
// so an operation never regrows a slice on the hot path; each count is
// pinned by a worst-case test, which is what keeps it honest when an
// attribute or a field is added.
const (
	// operationStartAttrCap is the most attributes an operation span carries
	// as it starts: component, operation, model, dry_run and batch_size.
	operationStartAttrCap = 5
	// operationOutcomeAttrCap is the most attributes the operation span's
	// outcome batch carries: the duration and one of record_not_found or
	// error.
	operationOutcomeAttrCap = 2
	// operationLogFieldCap is the most fields the operation's log entry
	// carries: model, batch_size, the duration pair, dry_run and one of
	// record_not_found or error.
	operationLogFieldCap = 5
	// hookOutcomeAttrCap is the most attributes a hook span's outcome batch
	// carries: the duration, success and error.
	hookOutcomeAttrCap = 3
)

// traceAs is trace with the operation's subject named by the caller: the
// model for an operation on one model, which trace passes, or the result row
// for a union, which reads several models through a chain borrowed from one
// of them. The subject is what the span and the log call the model.
func (db *database[M]) traceAs(modelName string, phase consts.Phase, batch ...int) (func(error), trace.Span) {
	begin := time.Now()
	var _batch int
	if len(batch) > 0 {
		_batch = batch[0]
	}

	ctx := db.ctx
	var span trace.Span
	if gstotel.IsEnabled() && ctx != nil {
		spanName := gstotel.FrameworkSpanName("database", modelName, phase.MethodName())
		ctx, span = gstotel.StartSpan(ctx, spanName)
		db.ctx = ctx

		// Update GORM database context with new span context
		db.ins = db.ins.WithContext(db.ctx)

		// Performance: submit all attributes of one phase in a single
		// SetAttributes call; every call on a recording span locks the span and
		// re-runs deduplication. When adding attributes, extend the batches in
		// this function instead of adding SetAttributes calls.
		if gstotel.IsSpanRecording(span) {
			attrs := make([]attribute.KeyValue, 0, operationStartAttrCap)
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

	// The comment reads the context as it is now, span included; comment.go
	// says why it is attached here rather than when the chain was built.
	db.attachStatementComment()

	return func(err error) {
		if span != nil {
			defer span.End()
		}

		// Record duration
		duration := time.Since(begin)

		// Update span with results if available; keep this a single batched
		// SetAttributes call (see the performance note above). The span and
		// the log below read the outcome the same way: success and
		// record-not-found are normal outcomes, so neither marks the span as
		// failed — a missing row is answered by an OK span carrying
		// database.record_not_found, the counterpart of the log's
		// record_not_found field — an operation its context canceled carries
		// database.canceled beside the log's canceled field, and only a real
		// failure records the error and sets the error status.
		if gstotel.IsSpanRecording(span) {
			attrs := make([]attribute.KeyValue, 0, operationOutcomeAttrCap)
			attrs = append(attrs, attribute.Int64("database.duration_ms", duration.Milliseconds()))

			switch {
			case err == nil:
				span.SetStatus(codes.Ok, "")
			case errors.Is(err, ErrRecordNotFound):
				span.SetStatus(codes.Ok, "")
				attrs = append(attrs, attribute.Bool("database.record_not_found", true))
			case errors.Is(err, context.Canceled):
				// Left unset: the operation did not complete, and did not fail
				// either — whoever canceled its context ended it.
				attrs = append(attrs, attribute.Bool("database.canceled", true))
			default:
				span.SetStatus(codes.Error, err.Error())
				gstotel.RecordError(span, err)
				attrs = append(attrs, attribute.Bool("error", true))
			}
			span.SetAttributes(attrs...)
		}

		// Log operation results. Success and record-not-found stay at debug
		// level: both are normal outcomes whose timing the SQL log and the
		// operation span already cover, so they only matter when tracing an
		// operation end to end. An operation its context canceled stays at
		// debug level too, since whoever canceled it ended it. Real failures
		// log at error level.
		fields := operationLogFields(modelName, _batch, duration, db.dryRun, err)
		switch {
		case err == nil || errors.Is(err, ErrRecordNotFound):
			logger.Database.WithContext(db.ctx, phase).Debugz("database operation completed", fields...)
		case errors.Is(err, context.Canceled):
			logger.Database.WithContext(db.ctx, phase).Debugz("database operation canceled", fields...)
		default:
			logger.Database.WithContext(db.ctx, phase).Errorz("database operation failed", fields...)
		}
	}, span
}

// operationLogFields builds the fields of one operation's log entry: the
// model, the constant markers batch_size and dry_run only when meaningful,
// the duration pair, and the outcome — record_not_found for a missing row,
// canceled for an operation its context canceled, the error for a failure,
// nothing for success.
func operationLogFields(modelName string, batch int, duration time.Duration, dryRun bool, err error) []zap.Field {
	fields := make([]zap.Field, 0, operationLogFieldCap)
	fields = append(fields, zap.String("model", modelName))
	if batch > 0 {
		fields = append(fields, zap.Int("batch_size", batch))
	}
	fields = append(fields, util.LogDuration(duration))
	if dryRun {
		fields = append(fields, zap.Bool("dry_run", true))
	}
	switch {
	case err == nil:
	case errors.Is(err, ErrRecordNotFound):
		fields = append(fields, zap.Bool("record_not_found", true))
	case errors.Is(err, context.Canceled):
		fields = append(fields, zap.Bool("canceled", true))
	default:
		fields = append(fields, zap.Error(err))
	}
	return fields
}

// traceModelHook runs one model hook under a span of its own, nested under
// the database operation span, and records its timing and outcome on it.
//
// The span is skipped and fn runs directly when tracing is disabled, when
// there is no parent span to nest under, or when the model does not override
// the hook: an unoverridden hook is the framework base's no-op, which still
// runs so the hook sequence stays the same for every model, but has nothing
// worth timing.
//
// Parameters:
//   - ctx: Database context the hook runs under; nil falls back to context.Background
//   - phase: Hook phase (consts.PHASE_CREATE_BEFORE, ...), naming the span "model.{Model}.{Hook}"
//   - parentSpan: Database operation span the hook span nests under
//   - fn: Hook invocation, receiving the context that carries the hook span
//
// Returns the hook's error, with the span completed either way.
func traceModelHook[M types.Model](ctx context.Context, phase consts.Phase, parentSpan trace.Span, fn func(ctx context.Context) error) error {
	hookCtx := context.Background()
	if ctx != nil {
		hookCtx = ctx
	}
	if !gstotel.IsEnabled() || ctx == nil || parentSpan == nil {
		return fn(hookCtx)
	}
	// An unoverridden hook is the framework base's no-op: it still runs, but
	// there is nothing to time.
	if !modelregistry.OverridesHook(*new(M), phase) {
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
		attrs := make([]attribute.KeyValue, 0, hookOutcomeAttrCap)
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
