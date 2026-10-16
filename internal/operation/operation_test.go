package operation_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/internal/operation"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

// TestBeginOpensARootSpanWhenTracingIsOn pins an operation's identity with
// tracing on: a root span named after the operation, on the context, and
// the trace id of that span.
func TestBeginOpensARootSpanWhenTracingIsOn(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)

	ctx, span, traceID := operation.Begin(context.Background(), "sample", "nightly-sweep")
	require.NotNil(t, span)
	span.End()

	require.Equal(t, span.SpanContext().TraceID().String(), traceID)
	require.Equal(t, span.SpanContext(), trace.SpanFromContext(ctx).SpanContext())
	ended := oteltest.EndedNamed(t, recorder, "sample.NightlySweep")
	require.Equal(t, traceID, ended.SpanContext().TraceID().String())
}

// TestBeginGeneratesATraceIDWhenTracingIsOff pins an operation's identity
// with tracing off: no span, and a generated W3C trace id.
func TestBeginGeneratesATraceIDWhenTracingIsOff(t *testing.T) {
	ctx, span, traceID := operation.Begin(context.Background(), "sample", "nightly-sweep")

	require.Nil(t, span)
	require.Regexp(t, `^[0-9a-f]{32}$`, traceID)
	require.False(t, trace.SpanFromContext(ctx).SpanContext().IsValid())
}
