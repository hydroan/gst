package otel_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
)

// TestTraceFunctionLeavesTheStatusOfACompletedRunUnset pins what the span of
// a traced function says about its outcome, the way instrumentation does
// under the OpenTelemetry convention: a run that returns no error leaves the
// status unset, with or without a result, and one that returns an error
// records it and marks the span failed.
func TestTraceFunctionLeavesTheStatusOfACompletedRunUnset(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	require.NoError(t, gstotel.TraceFunction(context.Background(), "sample.completed", func(context.Context) error { return nil }))
	result, err := gstotel.TraceFunctionWithResult(context.Background(), "sample.completed_with_result", func(context.Context) (int, error) { return 42, nil })
	require.NoError(t, err)
	require.Equal(t, 42, result)
	failure := errors.New("sample failure")
	require.ErrorIs(t, gstotel.TraceFunction(context.Background(), "sample.failed", func(context.Context) error { return failure }), failure)

	for _, name := range []string{"sample.completed", "sample.completed_with_result"} {
		span := oteltest.EndedNamed(t, recorder, name)
		require.Equal(t, codes.Unset, span.Status().Code, name)
		require.Empty(t, span.Events(), name)
	}
	failed := oteltest.EndedNamed(t, recorder, "sample.failed")
	require.Equal(t, codes.Error, failed.Status().Code)
	require.NotEmpty(t, failed.Events(), "the failure is recorded on the span")
}
