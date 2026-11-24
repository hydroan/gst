package execctx_test

import (
	"context"
	"strings"
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

// TestNewTraceIDAndNewSpanIDAreW3CIDs pins the form of the ids the framework
// generates for an execution outside any trace: a trace id of 16 random
// bytes and a span id of 8, in lower-case hex and never all zero, the form
// the W3C trace context defines and the X-Trace-ID header carries.
func TestNewTraceIDAndNewSpanIDAreW3CIDs(t *testing.T) {
	first, second := execctx.NewTraceID(), execctx.NewTraceID()
	require.Regexp(t, `^[0-9a-f]{32}$`, first)
	require.NotEqual(t, strings.Repeat("0", 32), first)
	require.NotEqual(t, first, second)

	span, other := execctx.NewSpanID(), execctx.NewSpanID()
	require.Regexp(t, `^[0-9a-f]{16}$`, span)
	require.NotEqual(t, strings.Repeat("0", 16), span)
	require.NotEqual(t, span, other)
}

func BenchmarkNewTraceID(b *testing.B) {
	for b.Loop() {
		execctx.NewTraceID()
	}
}
