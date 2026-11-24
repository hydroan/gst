package database

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/model"
	gstotel "github.com/hydroan/gst/internal/otel"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/logger"
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
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	db := &database[*traceSample]{ins: DB(), ctx: context.Background()}
	done, _ := db.traceAs("TraceSample", apiaction.Create, 3)
	done(errors.New("sample failure"))

	span := oteltest.EndedNamed(t, recorder, "database.TraceSample.Create")
	// Each batch is pinned by the count under its own keys, so one batch
	// growing past its capacity is not hidden by the other shrinking.
	batches := oteltest.AttributesByKey(t, span.Attributes(),
		[]attribute.Key{"component", "database.operation", "database.model", "database.dry_run", "database.batch_size"},
		[]attribute.Key{"database.duration_ms", "database.record_not_found", "database.stale_object", "database.canceled", "database.refused", "error"},
	)
	require.Len(t, batches[0], operationStartAttrCap, "the worst case must fill the start batch exactly: a new attribute bumps operationStartAttrCap")
	require.Len(t, batches[1], operationOutcomeAttrCap, "the worst case must fill the outcome batch exactly: a new attribute bumps operationOutcomeAttrCap")
}

// TestOperationLogFieldsFitTheCapacityInTheWorstCase pins operationLogFieldCap
// to the entry of a batched dry run that failed — every optional field
// present — so a field added without bumping the capacity fails here instead
// of regrowing the slice on every operation.
func TestOperationLogFieldsFitTheCapacityInTheWorstCase(t *testing.T) {
	fields := operationLogFields("TraceSample", 3, time.Millisecond, true, errors.New("sample failure"))

	require.Len(t, fields, operationLogFieldCap, "the worst case must fill the capacity exactly: a new field bumps operationLogFieldCap")
	require.Equal(t, operationLogFieldCap, cap(fields), "the slice must not have regrown")
}

// TestOperationLogFieldsReadTheOutcome pins the outcome field of an
// operation's log entry: record_not_found for a missing row, stale_object
// for a versioned write that matched none, canceled for an operation its
// context canceled, refused for one a model hook refused, and the error for
// a failure alone.
func TestOperationLogFieldsReadTheOutcome(t *testing.T) {
	keys := func(err error) map[string]bool {
		got := make(map[string]bool)
		for _, field := range operationLogFields("TraceSample", 0, time.Millisecond, false, err) {
			got[field.Key] = true
		}
		return got
	}

	require.True(t, keys(ErrRecordNotFound)["record_not_found"])
	stale := keys(errors.Wrap(ErrStaleObject, "update sample"))
	require.True(t, stale["stale_object"])
	require.False(t, stale["error"], "a stale object carries no error")
	canceled := keys(errors.Wrap(context.Canceled, "sample"))
	require.True(t, canceled["canceled"])
	require.False(t, canceled["error"], "a canceled operation carries no error")
	refused := keys(gst.NewError(http.StatusForbidden, "not yours"))
	require.True(t, refused["refused"])
	require.False(t, refused["error"], "an operation a hook refused carries no error")
	require.True(t, keys(errors.New("sample failure"))["error"])
}

// loggedOutcome is the level and message of one entry an operation logged.
type loggedOutcome struct {
	level zapcore.Level
	msg   string
}

// outcomeLogger stands in for the database stream's logger and records the
// level and message of each entry an operation logs through it.
type outcomeLogger struct {
	gst.Logger
	logged *[]loggedOutcome
}

func (l outcomeLogger) WithContext(context.Context, apiaction.Phase) gst.Logger { return l }

func (l outcomeLogger) Debugz(msg string, _ ...zap.Field) {
	*l.logged = append(*l.logged, loggedOutcome{zapcore.DebugLevel, msg})
}

func (l outcomeLogger) Errorz(msg string, _ ...zap.Field) {
	*l.logged = append(*l.logged, loggedOutcome{zapcore.ErrorLevel, msg})
}

// TestOperationLogLevelFollowsTheOutcome pins the level an operation's entry
// is logged at: debug for an operation that completed, the missing row and
// the stale object among the outcomes it completes with, for one its context
// canceled, and for one a model hook refused, a service error answering
// below 500; error for a failure alone, a hook's service error of 500 or
// above among them.
func TestOperationLogLevelFollowsTheOutcome(t *testing.T) {
	var logged []loggedOutcome
	original := logger.Database
	logger.Database = outcomeLogger{Logger: original, logged: &logged}
	t.Cleanup(func() { logger.Database = original })

	for _, err := range []error{
		nil,
		errors.Wrap(ErrRecordNotFound, "get sample"),
		errors.Wrap(ErrStaleObject, "update sample"),
		errors.Wrap(context.Canceled, "list samples"),
		gst.NewError(http.StatusForbidden, "not yours"),
		gst.NewError(http.StatusInternalServerError, "broken"),
		errors.New("sample failure"),
	} {
		db := &database[*traceSample]{ins: DB(), ctx: context.Background()}
		done, _ := db.traceAs("TraceSample", apiaction.Update)
		done(err)
	}

	require.Equal(t, []loggedOutcome{
		{zapcore.DebugLevel, "database operation completed"},
		{zapcore.DebugLevel, "database operation completed"},
		{zapcore.DebugLevel, "database operation completed"},
		{zapcore.DebugLevel, "database operation canceled"},
		{zapcore.DebugLevel, "database operation refused"},
		{zapcore.ErrorLevel, "database operation failed"},
		{zapcore.ErrorLevel, "database operation failed"},
	}, logged)
}

// TestAnOperationAHookRefusedIsNoFailureOfTheDatabase pins what the spans of
// the database layer make of a model hook's error. A refusal, a service
// error answering below 500 that the hook chose for the client, marks
// neither the operation's span nor the hook's failed and records no error
// on them: the operation's carries database.refused, and the hook's reports
// the hook unsuccessful. An error that is the server's own failure is
// recorded on both and marks both failed.
func TestAnOperationAHookRefusedIsNoFailureOfTheDatabase(t *testing.T) {
	attributes := func(span trace.ReadOnlySpan) map[attribute.Key]attribute.Value {
		attrs := make(map[attribute.Key]attribute.Value, len(span.Attributes()))
		for _, attr := range span.Attributes() {
			attrs[attr.Key] = attr.Value
		}
		return attrs
	}
	run := func(t *testing.T, hookErr error) (operation, hook trace.ReadOnlySpan) {
		t.Helper()
		oteltest.Enable(t)
		recorder := oteltest.Record(t)

		db := &database[*traceSample]{ins: DB(), ctx: context.Background()}
		done, span := db.traceAs("TraceSample", apiaction.Create)
		err := traceModelHook[*traceSample](db.ctx, apiaction.CreateBefore, span, func(context.Context) error { return hookErr })
		done(err)

		require.Same(t, hookErr, err)
		return oteltest.EndedNamed(t, recorder, "database.TraceSample.Create"), oteltest.EndedNamed(t, recorder, "model.TraceSample.CreateBefore")
	}

	t.Run("a refusal leaves both spans unset", func(t *testing.T) {
		operation, hook := run(t, gst.NewError(http.StatusForbidden, "not yours"))

		require.Equal(t, codes.Unset, operation.Status().Code)
		require.Empty(t, operation.Events(), "a refusal must not be recorded as an exception")
		require.True(t, attributes(operation)["database.refused"].AsBool())
		require.NotContains(t, attributes(operation), attribute.Key("error"))

		require.Equal(t, codes.Unset, hook.Status().Code)
		require.Empty(t, hook.Events(), "a refusal must not be recorded as an exception")
		require.False(t, attributes(hook)["model.success"].AsBool(), "a hook that refused did not succeed")
		require.NotContains(t, attributes(hook), attribute.Key("error"))
	})

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
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	ctx, parent := gstotel.StartSpan(context.Background(), "sample.parent")
	err := traceModelHook[*traceSample](ctx, apiaction.CreateBefore, parent, func(context.Context) error {
		return errors.New("sample hook failure")
	})
	parent.End()
	require.Error(t, err)

	span := oteltest.EndedNamed(t, recorder, "model.TraceSample.CreateBefore")
	// component, model.model and model.phase are set as the span starts; the
	// outcome batch adds the rest.
	const startAttrs = 3
	require.Len(t, span.Attributes(), startAttrs+hookOutcomeAttrCap,
		"the worst case must fill the outcome batch exactly: a new attribute bumps hookOutcomeAttrCap")
}
