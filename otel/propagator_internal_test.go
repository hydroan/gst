package otel

import (
	"context"
	"net/http"
	"testing"

	"github.com/hydroan/gst/internal/consts"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// The trace id the propagator tests send, and the span id they send beside
// it when the case has one.
const (
	sentTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	sentSpanID  = "00f067aa0ba902b7"
)

// TestTraceIDHeaderPropagatorExtractsTheHeaderAsTheParent pins what the
// propagator makes of the framework's own headers: a valid X-Trace-ID
// becomes the remote, sampled parent of the request, with the X-Span-ID
// sent beside it or, without one or with one that is no span id, the
// default span id; a trace context an earlier propagator found is kept over
// the header; and a header that is no trace id leaves the context as it is.
func TestTraceIDHeaderPropagatorExtractsTheHeaderAsTheParent(t *testing.T) {
	carrier := func(traceID, spanID string) propagation.HeaderCarrier {
		header := http.Header{}
		if traceID != "" {
			header.Set(consts.HEADER_TRACE_ID, traceID)
		}
		if spanID != "" {
			header.Set(consts.HEADER_SPAN_ID, spanID)
		}
		return propagation.HeaderCarrier(header)
	}

	t.Run("a trace id with a span id", func(t *testing.T) {
		sc := trace.SpanContextFromContext(traceIDHeaderPropagator{}.Extract(context.Background(), carrier(sentTraceID, sentSpanID)))

		require.True(t, sc.IsValid())
		require.True(t, sc.IsRemote())
		require.True(t, sc.IsSampled())
		require.Equal(t, sentTraceID, sc.TraceID().String())
		require.Equal(t, sentSpanID, sc.SpanID().String())
	})

	t.Run("a trace id alone takes the default span id", func(t *testing.T) {
		sc := trace.SpanContextFromContext(traceIDHeaderPropagator{}.Extract(context.Background(), carrier(" "+sentTraceID+" ", "")))

		require.Equal(t, sentTraceID, sc.TraceID().String())
		require.Equal(t, defaultSpanID, sc.SpanID())
	})

	t.Run("a span id that is none takes the default span id", func(t *testing.T) {
		sc := trace.SpanContextFromContext(traceIDHeaderPropagator{}.Extract(context.Background(), carrier(sentTraceID, "not-a-span-id")))

		require.Equal(t, sentTraceID, sc.TraceID().String())
		require.Equal(t, defaultSpanID, sc.SpanID())
	})

	t.Run("a trace context found before wins", func(t *testing.T) {
		found := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID: trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanID:  trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
			Remote:  true,
		})
		ctx := trace.ContextWithRemoteSpanContext(context.Background(), found)

		require.Equal(t, found, trace.SpanContextFromContext(traceIDHeaderPropagator{}.Extract(ctx, carrier(sentTraceID, sentSpanID))))
	})

	t.Run("a trace id that is none is ignored", func(t *testing.T) {
		require.False(t, trace.SpanContextFromContext(traceIDHeaderPropagator{}.Extract(context.Background(), carrier("not-a-trace-id", sentSpanID))).IsValid())
		require.False(t, trace.SpanContextFromContext(traceIDHeaderPropagator{}.Extract(context.Background(), carrier("", ""))).IsValid())
	})
}

// TestTraceIDHeaderPropagatorInjectsNothing pins that the propagator writes
// no header on the way out, the W3C propagators before it carrying the
// trace context and the framework's client setting the header itself, and
// that it names the two headers it reads as its fields.
func TestTraceIDHeaderPropagatorInjectsNothing(t *testing.T) {
	ctx := traceIDHeaderPropagator{}.Extract(context.Background(), propagation.MapCarrier{consts.HEADER_TRACE_ID: sentTraceID})
	carrier := propagation.MapCarrier{}

	traceIDHeaderPropagator{}.Inject(ctx, carrier)

	require.Empty(t, carrier)
	require.Equal(t, []string{consts.HEADER_TRACE_ID, consts.HEADER_SPAN_ID}, traceIDHeaderPropagator{}.Fields())
}
