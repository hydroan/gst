package execctx_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/internal/execctx"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestFromContextReturnsStampedTraceID(t *testing.T) {
	ctx := execctx.WithTraceID(context.Background(), "trace-1")

	require.Equal(t, execctx.Identity{TraceID: "trace-1"}, execctx.FromContext(ctx))
}

func TestFromContextReturnsCronjobRound(t *testing.T) {
	ctx := execctx.WithCronjob(context.Background(), "sample_job", "trace-2")

	require.Equal(t, execctx.Identity{TraceID: "trace-2", Cronjob: "sample_job"}, execctx.FromContext(ctx))
}

func TestFromContextReturnsLeaderTenure(t *testing.T) {
	ctx := execctx.WithLeader(context.Background(), "sample_work", "trace-3")

	require.Equal(t, execctx.Identity{TraceID: "trace-3", Leader: "sample_work"}, execctx.FromContext(ctx))
}

func TestFromContextBorrowsSpanTraceID(t *testing.T) {
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext(t, "11111111111111111111111111111111"))

	require.Equal(t, execctx.Identity{TraceID: "11111111111111111111111111111111"}, execctx.FromContext(ctx))
}

func TestFromContextPrefersStampOverSpan(t *testing.T) {
	// The stamp is what the middleware published to the caller; a span opened
	// later must not change the id the annotations carry.
	ctx := execctx.WithTraceID(context.Background(), "trace-1")
	ctx = trace.ContextWithSpanContext(ctx, spanContext(t, "22222222222222222222222222222222"))

	require.Equal(t, execctx.Identity{TraceID: "trace-1"}, execctx.FromContext(ctx))
}

func TestFromContextIsZeroWithoutIdentity(t *testing.T) {
	require.Equal(t, execctx.Identity{}, execctx.FromContext(context.Background()))
	require.Equal(t, execctx.Identity{}, execctx.FromContext(nil)) //nolint:staticcheck // nil is a supported input, mirroring requestctx.FromContext.
}

// spanContext builds a valid span context carrying the given trace id, the
// shape a remote parent or an open span leaves on a context.
func spanContext(t *testing.T, traceID string) trace.SpanContext {
	t.Helper()
	id, err := trace.TraceIDFromHex(traceID)
	require.NoError(t, err)
	return trace.NewSpanContext(trace.SpanContextConfig{TraceID: id, SpanID: trace.SpanID{1}})
}
