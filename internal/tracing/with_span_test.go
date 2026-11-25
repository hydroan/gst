package tracing_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/testutil/tracingtest"
	"github.com/hydroan/gst/internal/tracing"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
)

// TestWithSpanLeavesTheStatusOfACompletedRunUnset pins what the span of a
// traced function says about its outcome, the way instrumentation does under
// the OpenTelemetry convention: a run that returns no error leaves the status
// unset, with or without a result, and one that returns an error records it
// and marks the span failed.
func TestWithSpanLeavesTheStatusOfACompletedRunUnset(t *testing.T) {
	tracingtest.Enable(t)
	recorder := tracingtest.Record(t)

	require.NoError(t, tracing.WithSpan(context.Background(), "sample.completed", func(context.Context) error { return nil }))
	result, err := tracing.WithSpanResult(context.Background(), "sample.completed_with_result", func(context.Context) (int, error) { return 42, nil })
	require.NoError(t, err)
	require.Equal(t, 42, result)
	failure := errors.New("sample failure")
	require.ErrorIs(t, tracing.WithSpan(context.Background(), "sample.failed", func(context.Context) error { return failure }), failure)

	for _, name := range []string{"sample.completed", "sample.completed_with_result"} {
		span := tracingtest.EndedNamed(t, recorder, name)
		require.Equal(t, codes.Unset, span.Status().Code, name)
		require.Empty(t, span.Events(), name)
	}
	failed := tracingtest.EndedNamed(t, recorder, "sample.failed")
	require.Equal(t, codes.Error, failed.Status().Code)
	require.NotEmpty(t, failed.Events(), "the failure is recorded on the span")
}
