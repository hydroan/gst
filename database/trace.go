package database

import (
	"context"
	"reflect"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/dbruntime/dberror"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logfield"
	"github.com/hydroan/gst/internal/logger"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Phases of database-layer operations that have no controller counterpart
// in apiaction. Values follow the apiaction.Phase snake_case convention so
// the log phase field stays uniform across every log source; span names
// derive their UpperCamelCase form via Phase.Name.
const (
	phaseUpsert        apiaction.Phase = "upsert"
	phaseCount         apiaction.Phase = "count"
	phaseFirst         apiaction.Phase = "first"
	phaseLast          apiaction.Phase = "last"
	phaseTake          apiaction.Phase = "take"
	phaseUpdateByID    apiaction.Phase = "update_by_id"
	phaseSelect        apiaction.Phase = "select"
	phaseSelectOne     apiaction.Phase = "select_one"
	phaseSelectCount   apiaction.Phase = "select_count"
	phaseUnionAll      apiaction.Phase = "union_all"
	phaseUnionAllCount apiaction.Phase = "union_all_count"
	phaseCleanup       apiaction.Phase = "cleanup"
	phaseHealth        apiaction.Phase = "health"
	phaseWithQuery     apiaction.Phase = "with_query"
	phaseTransaction   apiaction.Phase = "transaction"
)

// trace returns a timing function for database operations that provides
// comprehensive performance monitoring, logging, and distributed tracing
// capabilities. The returned function should be called with the operation
// result to complete tracing and logging.
//
// Parameters:
//   - phase: Operation phase for logging and tracing identification
//     (apiaction.Create, apiaction.List, phaseUpsert, etc.)
//   - batch: Optional batch size for batch operations (used for span attributes
//     and logging)
//
// Returns a function that accepts an error and completes the operation tracing
// and logging.
//
// Features:
//   - Automatic timing measurement from call to completion
//   - OTEL distributed tracing integration with OpenTelemetry spans
//   - Comprehensive span attributes including operation metadata
//   - A log entry for a failure alone and span status management to match:
//     a missing row and a stale object are outcomes an operation completes
//     with, and an operation its context canceled or one refused, by a model
//     hook or over the data it carries, is no failure (see operationOutcome)
//   - Batch operation support with size tracking
//   - Dry-run mode status recording
//   - Smart duration formatting for readability
//   - Context propagation to GORM operations
//
// OTEL Tracing Integration:
//   - Creates OpenTelemetry spans with naming pattern:
//     "database.{Model}.{Operation}"
//   - Records detailed span attributes: component, operation, model, table,
//     batch_size, etc.
//   - Propagates span context to GORM operations for complete tracing hierarchy
//   - Automatically handles span lifecycle (creation, attribute setting,
//     completion)
//   - Integrates with existing tracing infrastructure (controller and service
//     layers)
//   - Ensures trace_id is available in database logs through request metadata
//     or the active OTEL span context
//
// Usage Pattern:
//
//	done, _ := db.trace(apiaction.Create, len(models))
//	defer func() { done(err) }()
//
// The closure is load-bearing: a plain `defer done(err)` evaluates err where
// the defer is registered, which is before the operation has run, so every
// span would be finished with a nil error and no failure would ever be
// recorded on it.
//
// Tracing Hierarchy:
//
//	HTTP or gRPC → Controller → Service → Database → GORM
//
// Note: Must be called after `defer db.reset()` to ensure proper cleanup order.
// Spans are recorded when tracing.IsEnabled() returns true.
func (db *database[M]) trace(phase apiaction.Phase, batch ...int) (func(error), trace.Span) {
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
	// outcome batch carries: the duration and one of record_not_found,
	// stale_object, canceled, refused or error.
	operationOutcomeAttrCap = 2
	// operationLogFieldCap is the most fields the entry of a failed operation
	// carries: model, batch_size, the duration pair, dry_run and the error.
	operationLogFieldCap = 5
	// hookOutcomeAttrCap is the most attributes a hook span's outcome batch
	// carries: the duration, success and error.
	hookOutcomeAttrCap = 3
)

// traceAs is trace with the operation's subject named by the caller: the
// model for an operation on one model, which trace passes, or the result row
// for a union, which reads several models through a chain borrowed from one
// of them. The subject is what the span and the log call the model.
func (db *database[M]) traceAs(modelName string, phase apiaction.Phase, batch ...int) (func(error), trace.Span) {
	begin := time.Now()
	var _batch int
	if len(batch) > 0 {
		_batch = batch[0]
	}

	ctx := db.ctx
	var span trace.Span
	if tracing.IsEnabled() && ctx != nil {
		spanName := tracing.ResourceSpanName("database", modelName, phase.Name())
		ctx, span = tracing.StartSpan(ctx, spanName)
		db.ctx = ctx

		// Update GORM database context with new span context
		db.ins = db.ins.WithContext(db.ctx)

		// Performance: submit all attributes of one phase in a single
		// SetAttributes call; every call on a recording span locks the span and
		// re-runs deduplication. When adding attributes, extend the batches in
		// this function instead of adding SetAttributes calls.
		if tracing.IsSpanRecording(span) {
			attrs := make([]attribute.KeyValue, 0, operationStartAttrCap)
			attrs = append(
				attrs,
				attribute.String("component", "database"),
				attribute.String("database.operation", phase.Name()),
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

		// The span and the log read the outcome once, the same way (see
		// operationOutcome). The span's results stay a single batched
		// SetAttributes call (see the performance note above): the duration
		// and the mark of the outcome, an operation that completed carrying
		// none. Only a failure records the error and sets the error status;
		// every other outcome leaves the status unset, as instrumentation
		// leaves an operation that completed.
		mark, failed := operationOutcome(err)
		if tracing.IsSpanRecording(span) {
			attrs := make([]attribute.KeyValue, 0, operationOutcomeAttrCap)
			attrs = append(attrs, attribute.Int64("database.duration_ms", duration.Milliseconds()))
			if failed {
				tracing.RecordError(span, err)
			}
			if len(mark) > 0 {
				attrs = append(attrs, attribute.Bool(mark, true))
			}
			span.SetAttributes(attrs...)
		}

		// The log records a failure alone, at error level. Every other
		// outcome is one the SQL log and the operation span already cover,
		// and none of them is the database layer's to report: an operation
		// that completed, one its context canceled, and one refused, which
		// whoever answers the client reports at the level of the answer.
		if failed {
			logger.Database.WithContext(db.ctx, phase).Errorz("database operation failed", operationLogFields(modelName, _batch, duration, db.dryRun, err)...)
		}
	}, span
}

// operationOutcome reads how an operation ended, the one reading its span
// and its log share: the attribute that marks the outcome on the span, none
// for an operation that completed, and whether the operation failed. A
// missing row and a versioned write that matched no row are outcomes an
// operation completes with, which the caller is told of to answer not found
// or to reload; an operation its context canceled did not complete and did
// not fail either, whoever canceled it having ended it; one refused (see
// refused) is no failure of the database; anything else is.
func operationOutcome(err error) (mark string, failed bool) {
	switch {
	case err == nil:
		return "", false
	case errors.Is(err, ErrRecordNotFound):
		return "database.record_not_found", false
	case errors.Is(err, ErrStaleObject):
		return "database.stale_object", false
	case errors.Is(err, context.Canceled):
		return "database.canceled", false
	case refused(err):
		return "database.refused", false
	default:
		return "error", true
	}
}

// refused reports whether err, what an operation or a model hook ended with,
// is a refusal rather than a failure of the database: a model hook refusing
// with a status of its choosing (see gst.ServerFailure), or a write refused
// over the data it carries (see dberror.IsDataIntegrityViolation). Whoever
// answers the client reports a refusal, at the level of the answer.
func refused(err error) bool {
	return !gst.ServerFailure(err) || dberror.IsDataIntegrityViolation(err)
}

// operationLogFields builds the fields of the entry a failed operation logs:
// the model, the constant markers batch_size and dry_run only when
// meaningful, the duration pair, and the error.
func operationLogFields(modelName string, batch int, duration time.Duration, dryRun bool, err error) []zap.Field {
	fields := make([]zap.Field, 0, operationLogFieldCap)
	fields = append(fields, zap.String("model", modelName))
	if batch > 0 {
		fields = append(fields, zap.Int("batch_size", batch))
	}
	fields = append(fields, logfield.Duration(duration))
	if dryRun {
		fields = append(fields, zap.Bool("dry_run", true))
	}
	return append(fields, zap.Error(err))
}

// traceModelHook runs one model hook under a span of its own, nested under
// the database operation span, and records its timing and outcome on it: the
// span is marked failed, the error recorded on it, for a failure alone, a
// refusal (see refused) leaving it unset.
//
// The span is skipped and fn runs directly when tracing is disabled, when
// there is no parent span to nest under, or when the model does not override
// the hook: an unoverridden hook is the framework base's no-op, which still
// runs so the hook sequence stays the same for every model, but has nothing
// worth timing.
//
// Parameters:
//   - ctx: Database context the hook runs under; nil falls back to
//     context.Background
//   - phase: Hook phase (apiaction.CreateBefore, ...), naming the span
//     "model.{Model}.{Hook}"
//   - parentSpan: Database operation span the hook span nests under
//   - fn: Hook invocation, receiving the context that carries the hook span
//
// Returns the hook's error, with the span completed either way.
func traceModelHook[M gst.Model](ctx context.Context, phase apiaction.Phase, parentSpan trace.Span, fn func(ctx context.Context) error) error {
	hookCtx := context.Background()
	if ctx != nil {
		hookCtx = ctx
	}
	if !tracing.IsEnabled() || ctx == nil || parentSpan == nil {
		return fn(hookCtx)
	}
	// An unoverridden hook is the framework base's no-op: it still runs, but
	// there is nothing to time.
	if !model.OverridesHook(*new(M), phase) {
		return fn(hookCtx)
	}

	modelName := reflect.TypeOf(*new(M)).Elem().Name()
	// Use a structured gst span name under the database span for hook
	// execution.
	spanName := tracing.ResourceSpanName("model", modelName, phase.Name())
	parentCtx := trace.ContextWithSpan(hookCtx, parentSpan)
	childCtx, span := tracing.StartSpan(parentCtx, spanName)
	defer span.End()

	recording := tracing.IsSpanRecording(span)
	var start time.Time
	if recording {
		// Add hook-specific attributes
		span.SetAttributes(
			attribute.String("component", "model"),
			attribute.String("model.model", modelName),
			attribute.String("model.phase", phase.Name()),
		)

		// Record start time
		start = time.Now()
	}

	// Execute hook function
	err := fn(childCtx)

	if recording {
		// Record execution results in a single batched SetAttributes call;
		// every call on a recording span locks the span and re-runs
		// deduplication.
		duration := time.Since(start)
		attrs := make([]attribute.KeyValue, 0, hookOutcomeAttrCap)
		attrs = append(
			attrs,
			attribute.Int64("model.duration_ms", duration.Milliseconds()),
			attribute.Bool("model.success", err == nil),
		)

		// Marked failed for a failure alone: a refusal, the hook's own with
		// a status of its choosing or that of a write the hook made, is no
		// failure of the hook.
		if err != nil && !refused(err) {
			tracing.RecordError(span, err)
			attrs = append(attrs, attribute.Bool("error", true))
		}
		span.SetAttributes(attrs...)
	}

	return err
}
