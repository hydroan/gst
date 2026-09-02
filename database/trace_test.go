package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestTraceModelHookSpansOnlyOverriddenHooks verifies that a model hook gets a
// span of its own only when the model overrides it: the framework base's no-op
// still runs, but there is nothing to time, so it must not export a span.
func TestTraceModelHookSpansOnlyOverriddenHooks(t *testing.T) {
	setupOTELTest(t)
	recorder := recordSpans(t)
	defer cleanupTestData()

	t.Run("an overridden hook gets a span and its no-op partner does not", func(t *testing.T) {
		defer cleanupTestData()
		// TestUser overrides CreateBefore and leaves CreateAfter to the framework base.
		user := &TestUser{Name: "trace-create", Email: "trace-create@example.com"}
		require.NoError(t, database.Database[*TestUser](context.Background()).Create(user))

		names := endedSpanNames(recorder)
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

		names := endedSpanNames(recorder)
		require.Contains(t, names, "database.TestItem.Get")
		for _, name := range names {
			require.False(t, strings.HasPrefix(name, "model.TestItem."), "no-op hook must not export span %q", name)
		}
	})
}

// recordSpans attaches an in-memory span recorder to the SDK tracer provider
// that setupOTELTest installed, so a test can read back the spans a database
// operation exported without any exporter of its own.
func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	provider, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	require.True(t, ok, "otel.Init must install the SDK tracer provider globally")
	recorder := tracetest.NewSpanRecorder()
	provider.RegisterSpanProcessor(recorder)
	return recorder
}

// endedSpanNames returns the names of every span the recorder has seen end.
func endedSpanNames(recorder *tracetest.SpanRecorder) []string {
	ended := recorder.Ended()
	names := make([]string, 0, len(ended))
	for _, span := range ended {
		names = append(names, span.Name())
	}
	return names
}
