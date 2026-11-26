package database

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/hydroan/gst/internal/testutil/tracingtest"
	"github.com/hydroan/gst/internal/tracing"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// traceSample is a model for the tracing helpers alone: never registered and
// never written, it only names the spans and overrides one hook so that the
// hook span exists.
type traceSample struct {
	model.Base
}

func (*traceSample) TableName() string { return "trace_samples" }

func (*traceSample) CreateBefore(context.Context) error { return nil }

// TestOperationSpanAttributesFitTheCapacityInTheWorstCase pins the two
// batches an operation span carries — the start batch of a batched
// operation, the outcome batch of a failed one — so an attribute added
// without bumping its capacity fails here instead of regrowing the slice on
// every operation.
func TestOperationSpanAttributesFitTheCapacityInTheWorstCase(t *testing.T) {
	tracingtest.Enable(t)
	recorder := tracingtest.Record(t)

	db := &database[*traceSample]{ins: DB(), ctx: context.Background()}
	done, _ := db.traceAs("TraceSample", apiaction.Create, 3)
	done(errors.New("sample failure"))

	span := tracingtest.EndedNamed(t, recorder, "database.TraceSample.Create")
	// Each batch is pinned by the count under its own keys, so one batch
	// growing past its capacity is not hidden by the other shrinking.
	batches := tracingtest.AttributesByKey(t, span.Attributes(),
		[]attribute.Key{"component", "database.operation", "database.model", "database.dry_run", "database.batch_size"},
		[]attribute.Key{"database.duration_ms", "database.record_not_found", "database.stale_object", "database.canceled", "database.refused", "error"},
	)
	require.Len(t, batches[0], operationStartAttrCap, "the worst case must fill the start batch exactly: a new attribute bumps operationStartAttrCap")
	require.Len(t, batches[1], operationOutcomeAttrCap, "the worst case must fill the outcome batch exactly: a new attribute bumps operationOutcomeAttrCap")
}

// TestOperationLogFieldsFitTheCapacityInTheWorstCase pins operationLogFieldCap
// to the entry of a batched dry run that failed — every optional field
// present — so a field added without bumping the capacity fails here instead
// of regrowing the slice.
func TestOperationLogFieldsFitTheCapacityInTheWorstCase(t *testing.T) {
	fields := operationLogFields("TraceSample", 3, time.Millisecond, true, errors.New("sample failure"))

	require.Len(t, fields, operationLogFieldCap, "the worst case must fill the capacity exactly: a new field bumps operationLogFieldCap")
	require.Equal(t, operationLogFieldCap, cap(fields), "the slice must not have regrown")
}

// loggedEntry is the level and message of one entry an operation logged.
type loggedEntry struct {
	level zapcore.Level
	msg   string
}

// entryLogger stands in for the database stream's logger and records the
// level and message of each entry an operation logs through it.
type entryLogger struct {
	gst.Logger
	logged *[]loggedEntry
}

func (l entryLogger) WithContext(context.Context, apiaction.Phase) gst.Logger { return l }

func (l entryLogger) Debugz(msg string, _ ...zap.Field) {
	*l.logged = append(*l.logged, loggedEntry{zapcore.DebugLevel, msg})
}

func (l entryLogger) Infoz(msg string, _ ...zap.Field) {
	*l.logged = append(*l.logged, loggedEntry{zapcore.InfoLevel, msg})
}

func (l entryLogger) Warnz(msg string, _ ...zap.Field) {
	*l.logged = append(*l.logged, loggedEntry{zapcore.WarnLevel, msg})
}

func (l entryLogger) Errorz(msg string, _ ...zap.Field) {
	*l.logged = append(*l.logged, loggedEntry{zapcore.ErrorLevel, msg})
}

// TestAnOperationLogsItsFailureAlone pins what an operation writes to the
// database stream. Nothing when it completed, the missing row and the stale
// object among the outcomes it completes with, when its context canceled it,
// or when it was refused, by a model hook with a service error answering
// below 500 or over the data it carries. One entry at error level when it
// failed, a hook's service error of 500 or above among the failures.
func TestAnOperationLogsItsFailureAlone(t *testing.T) {
	var logged []loggedEntry
	original := logger.Database
	logger.Database = entryLogger{Logger: original, logged: &logged}
	t.Cleanup(func() { logger.Database = original })

	for _, err := range []error{
		nil,
		errors.Wrap(ErrRecordNotFound, "get sample"),
		errors.Wrap(ErrStaleObject, "update sample"),
		errors.Wrap(context.Canceled, "list samples"),
		gst.NewError(http.StatusForbidden, "not yours"),
		errors.Wrap(ErrDuplicatedKey, "create sample"),
		errors.Wrap(ErrForeignKeyViolated, "create sample"),
		errors.Wrap(ErrCheckConstraintViolated, "create sample"),
		errors.Wrap(ErrValueTooLong, "create sample"),
		errors.Wrap(ErrNotNullViolated, "create sample"),
		errors.Wrap(tenant.ErrImmutable, "update sample"),
		errors.Wrap(tenant.ErrRequired, "create sample"),
		gst.NewError(http.StatusInternalServerError, "broken"),
		errors.New("sample failure"),
	} {
		db := &database[*traceSample]{ins: DB(), ctx: context.Background()}
		done, _ := db.traceAs("TraceSample", apiaction.Update)
		done(err)
	}

	require.Equal(t, []loggedEntry{
		{zapcore.ErrorLevel, "database operation failed"},
		{zapcore.ErrorLevel, "database operation failed"},
	}, logged)
}

// TestARefusedOperationIsNoFailureOfTheDatabase pins what the spans of the
// database layer make of the error a model hook hands an operation. A
// refusal, a service error answering below 500 that the hook chose for the
// client or a write refused over the data it carries, marks neither the
// operation's span nor the hook's failed and records no error on them: the
// operation's carries database.refused, and the hook's reports the hook
// unsuccessful. An error that is the server's own failure is recorded on
// both and marks both failed.
func TestARefusedOperationIsNoFailureOfTheDatabase(t *testing.T) {
	attributes := func(span trace.ReadOnlySpan) map[attribute.Key]attribute.Value {
		attrs := make(map[attribute.Key]attribute.Value, len(span.Attributes()))
		for _, attr := range span.Attributes() {
			attrs[attr.Key] = attr.Value
		}
		return attrs
	}
	run := func(t *testing.T, hookErr error) (operation, hook trace.ReadOnlySpan) {
		t.Helper()
		tracingtest.Enable(t)
		recorder := tracingtest.Record(t)

		db := &database[*traceSample]{ins: DB(), ctx: context.Background()}
		done, span := db.traceAs("TraceSample", apiaction.Create)
		err := traceModelHook[*traceSample](db.ctx, apiaction.CreateBefore, span, func(context.Context) error { return hookErr })
		done(err)

		require.Same(t, hookErr, err)
		return tracingtest.EndedNamed(t, recorder, "database.TraceSample.Create"), tracingtest.EndedNamed(t, recorder, "model.TraceSample.CreateBefore")
	}

	for name, refusal := range map[string]error{
		"a hook's refusal":                  gst.NewError(http.StatusForbidden, "not yours"),
		"a key another row holds":           errors.Wrap(ErrDuplicatedKey, "create sample"),
		"a value the table's check refuses": errors.Wrap(ErrCheckConstraintViolated, "create sample"),
		"a tenant the row cannot have":      errors.Wrap(tenant.ErrImmutable, "update sample"),
	} {
		t.Run(name+" leaves both spans unset", func(t *testing.T) {
			operation, hook := run(t, refusal)

			require.Equal(t, codes.Unset, operation.Status().Code)
			require.Empty(t, operation.Events(), "a refusal must not be recorded as an exception")
			require.True(t, attributes(operation)["database.refused"].AsBool())
			require.NotContains(t, attributes(operation), attribute.Key("error"))

			require.Equal(t, codes.Unset, hook.Status().Code)
			require.Empty(t, hook.Events(), "a refusal must not be recorded as an exception")
			require.False(t, attributes(hook)["model.success"].AsBool(), "a hook that refused did not succeed")
			require.NotContains(t, attributes(hook), attribute.Key("error"))
		})
	}

	t.Run("the server's own failure marks both spans failed", func(t *testing.T) {
		operation, hook := run(t, errors.New("sample hook failure"))

		require.Equal(t, codes.Error, operation.Status().Code)
		require.Len(t, operation.Events(), 1, "the error is recorded as one exception event")
		require.True(t, attributes(operation)["error"].AsBool())
		require.NotContains(t, attributes(operation), attribute.Key("database.refused"))

		require.Equal(t, codes.Error, hook.Status().Code)
		require.Len(t, hook.Events(), 1, "the error is recorded as one exception event")
		require.True(t, attributes(hook)["error"].AsBool())
	})
}

// TestHookSpanAttributesFitTheCapacityInTheWorstCase pins hookOutcomeAttrCap
// to the outcome batch of a hook that failed, on top of the three attributes
// the hook span starts with.
func TestHookSpanAttributesFitTheCapacityInTheWorstCase(t *testing.T) {
	tracingtest.Enable(t)
	recorder := tracingtest.Record(t)

	ctx, parent := tracing.StartSpan(context.Background(), "sample.parent")
	err := traceModelHook[*traceSample](ctx, apiaction.CreateBefore, parent, func(context.Context) error {
		return errors.New("sample hook failure")
	})
	parent.End()
	require.Error(t, err)

	span := tracingtest.EndedNamed(t, recorder, "model.TraceSample.CreateBefore")
	// component, model.model and model.phase are set as the span starts; the
	// outcome batch adds the rest.
	const startAttrs = 3
	require.Len(t, span.Attributes(), startAttrs+hookOutcomeAttrCap,
		"the worst case must fill the outcome batch exactly: a new attribute bumps hookOutcomeAttrCap")
}
