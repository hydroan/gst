package tracing_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/testutil/tracingtest"
	"github.com/hydroan/gst/tracing"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
)

// TestTheForwardedFunctionsTraceOnTheFrameworksProvider pins that the public
// package is the framework's tracing and not a second one beside it: a span a
// project starts through it ends on the tracer provider the framework
// initialized, under the name SpanName builds, carrying the error RecordError
// recorded; and a function WithSpan or WithSpanResult runs is traced under
// the span its context carries.
func TestTheForwardedFunctionsTraceOnTheFrameworksProvider(t *testing.T) {
	tracingtest.Enable(t)
	recorder := tracingtest.Record(t)

	name := tracing.SpanName("sample", "import_rows")
	require.Equal(t, "sample.ImportRows", name)

	ctx, span := tracing.StartSpan(context.Background(), name)
	failure := errors.New("sample failure")
	tracing.RecordError(span, failure)
	require.NoError(t, tracing.WithSpan(ctx, "sample.function", func(context.Context) error { return nil }))
	result, err := tracing.WithSpanResult(ctx, "sample.function_with_result", func(context.Context) (int, error) { return 42, nil })
	require.NoError(t, err)
	require.Equal(t, 42, result)
	span.End()

	started := tracingtest.EndedNamed(t, recorder, name)
	require.Equal(t, codes.Error, started.Status().Code)
	require.Equal(t, failure.Error(), started.Status().Description)
	for _, function := range []string{"sample.function", "sample.function_with_result"} {
		traced := tracingtest.EndedNamed(t, recorder, function)
		require.Equal(t, started.SpanContext().SpanID(), traced.Parent().SpanID(), function)
	}
}
