package database_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestTraceModelHookSpansOnlyOverriddenHooks verifies that a model hook gets a
// span of its own only when the model overrides it: the framework base's no-op
// still runs, but there is nothing to time, so it must not export a span.
func TestTraceModelHookSpansOnlyOverriddenHooks(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	defer cleanupTestData()

	t.Run("an overridden hook gets a span and its no-op partner does not", func(t *testing.T) {
		defer cleanupTestData()
		// TestUser overrides CreateBefore and leaves CreateAfter to the framework base.
		user := &TestUser{Name: "trace-create", Email: "trace-create@example.com"}
		require.NoError(t, database.Database[*TestUser](context.Background()).Create(user))

		names := oteltest.EndedNames(recorder)
		require.Contains(t, names, "database.TestUser.Create")
		require.Contains(t, names, "model.TestUser.CreateBefore")
		require.NotContains(t, names, "model.TestUser.CreateAfter")
	})

	t.Run("a model without hook overrides gets no hook span", func(t *testing.T) {
		defer cleanupTestData()
		item := &TestItem{Name: "trace-get"}
		require.NoError(t, database.Database[*TestItem](context.Background()).Create(item))
		stored := new(TestItem)
		require.NoError(t, database.Database[*TestItem](context.Background()).Get(stored, item.ID))

		names := oteltest.EndedNames(recorder)
		require.Contains(t, names, "database.TestItem.Get")
		for _, name := range names {
			require.False(t, strings.HasPrefix(name, "model.TestItem."), "no-op hook must not export span %q", name)
		}
	})
}

// TestTraceSpanReadsOutcomeLikeTheLog pins that the operation span reads an
// outcome the way the log does: a missing row is a normal outcome, answered
// by an OK span carrying database.record_not_found and no recorded error,
// while a real failure records the error and marks the span failed.
func TestTraceSpanReadsOutcomeLikeTheLog(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	defer cleanupTestData()

	t.Run("a missing row leaves the span ok and marks it record_not_found", func(t *testing.T) {
		stored := new(TestItem)
		err := database.Database[*TestItem](context.Background()).Get(stored, "no-such-item")
		require.ErrorIs(t, err, database.ErrRecordNotFound)

		span := lastEndedNamed(t, recorder, "database.TestItem.Get")
		require.Equal(t, codes.Ok, span.Status().Code)
		require.Empty(t, span.Events(), "a missing row must not be recorded as an exception")
		attrs := spanAttributes(span)
		notFound, ok := attrs["database.record_not_found"].(bool)
		require.True(t, ok && notFound, "the span must carry database.record_not_found")
		require.NotContains(t, attrs, "error")
	})

	t.Run("a real failure records the error and marks the span failed", func(t *testing.T) {
		code := "trace-dup-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		first := &TestUniqueItem{UniqueCode: code, Name: "first"}
		require.NoError(t, database.Database[*TestUniqueItem](context.Background()).Create(first))
		second := &TestUniqueItem{UniqueCode: code, Name: "second"}
		require.ErrorIs(t, database.Database[*TestUniqueItem](context.Background()).Create(second), database.ErrDuplicatedKey)

		span := lastEndedNamed(t, recorder, "database.TestUniqueItem.Create")
		require.Equal(t, codes.Error, span.Status().Code)
		require.NotEmpty(t, span.Events(), "a real failure must be recorded on the span")
		attrs := spanAttributes(span)
		failed, ok := attrs["error"].(bool)
		require.True(t, ok && failed, "the span must carry the error marker")
		require.NotContains(t, attrs, "database.record_not_found")
	})
}

// lastEndedNamed returns the most recently ended span carrying name, which is
// the one an operation repeated in a test just finished.
func lastEndedNamed(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range slices.Backward(recorder.Ended()) {
		if span.Name() == name {
			return span
		}
	}
	require.FailNow(t, "no ended span named "+name)
	return nil
}

// spanAttributes indexes a span's attributes by key.
func spanAttributes(span sdktrace.ReadOnlySpan) map[string]any {
	attrs := make(map[string]any, len(span.Attributes()))
	for _, kv := range span.Attributes() {
		attrs[string(kv.Key)] = kv.Value.AsInterface()
	}
	return attrs
}

// TestTraceSelectPhasesNameTheSpans pins the phases the select and union
// terminals trace under: the span names, and the log phase field they
// derive from, are what dashboards and log searches key on.
func TestTraceSelectPhasesNameTheSpans(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	defer cleanupAggregateData()
	defer cleanupTagData()
	setupAggregateData(t)
	setupTagData(t)
	ctx := context.Background()

	type total struct{ Total int64 }
	type perCategory struct {
		Category string
		Total    int64
	}
	rows := make([]perCategory, 0)
	require.NoError(t, database.Select[*TestAggregateRecord, perCategory](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).Scan(&rows))
	one := total{}
	require.NoError(t, database.Select[*TestAggregateRecord, total](ctx, TestAggregateRecordCols.Amount.Sum().As("total")).ScanOne(&one))
	groups := 0
	require.NoError(t, database.Select[*TestAggregateRecord, perCategory](ctx, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).Count(&groups))
	feed := make([]feedRow, 0)
	require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).Scan(&feed))
	stacked := 0
	require.NoError(t, database.UnionAll[feedRow](ctx, recordsBranch(ctx), tagsBranch(ctx)).Count(&stacked))

	names := oteltest.EndedNames(recorder)
	require.Contains(t, names, "database.TestAggregateRecord.Select")
	require.Contains(t, names, "database.TestAggregateRecord.SelectOne")
	require.Contains(t, names, "database.TestAggregateRecord.SelectCount")
	ends := func(suffix string) bool {
		for _, name := range names {
			if strings.HasSuffix(name, suffix) {
				return true
			}
		}
		return false
	}
	require.True(t, ends(".UnionAll"), "the union stacks under union_all: %v", names)
	require.True(t, ends(".UnionAllCount"), "the union counts under union_all_count: %v", names)
}
