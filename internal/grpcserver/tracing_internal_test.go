package grpcserver

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// traced is what a call's handler found on its context: the trace id of the
// execution and the span context of the call.
type traced struct {
	traceID string
	span    trace.SpanContext
}

// TestCallsAreTracedWithTracingOn pins the gRPC counterpart of the HTTP
// listener's tracing middleware. With tracing on, a call runs inside a server
// span named by its full method and carrying the rpc attributes of the
// semantic conventions; the trace id the handler, the access log and the
// response header see is the span's; the caller's W3C trace context or,
// when it sent none, a valid x-trace-id seeds the span, the way the HTTP
// listener reads the traceparent and X-Trace-ID headers; a call's status
// code ends up on the span, a refusal leaving the status unset and a
// failure of the server's own setting it to error; and a panic is recorded
// on the span the way the HTTP recovery middleware records one.
func TestCallsAreTracedWithTracingOn(t *testing.T) {
	reset(t)
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	seen := make(chan traced, 1)
	report := func(ctx context.Context) error {
		seen <- traced{traceID: execctx.FromContext(ctx).TraceID, span: trace.SpanContextFromContext(ctx)}
		return nil
	}
	serve(map[string]func(context.Context) error{
		"Look":   report,
		"Seeded": report,
		"Quoted": report,
		"Deny":   func(context.Context) error { return status.Error(codes.PermissionDenied, "not yours") },
		"Boom":   func(context.Context) error { panic("sample panic") },
	})
	addr := start(t)
	conn := dial(t, addr, nil)

	t.Run("a call runs in a server span whose trace id it carries", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var header metadata.MD

		require.NoError(t, call(ctx, conn, "Look", grpc.Header(&header)))

		got := <-seen
		span := oteltest.EndedNamed(t, recorder, "gst.test.Echo/Look")
		require.Equal(t, trace.SpanKindServer, span.SpanKind())
		require.True(t, got.span.IsValid(), "the handler runs inside the span")
		require.Equal(t, span.SpanContext().TraceID(), got.span.TraceID())
		require.Equal(t, span.SpanContext().TraceID().String(), got.traceID, "the execution's trace id is the span's")
		require.Equal(t, []string{got.traceID}, header.Get("x-trace-id"))
		// The attributes are the ones the semantic conventions otelgrpc
		// follows name: the rpc.method is the full method.
		attrs := attributesOf(span)
		require.Equal(t, "grpc", attrs["rpc.system.name"])
		require.Equal(t, "gst.test.Echo/Look", attrs["rpc.method"])
		require.Equal(t, "OK", attrs["rpc.response.status_code"])
		require.Equal(t, otelcodes.Unset, span.Status().Code)
	})

	t.Run("the caller's trace context seeds the span", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = metadata.AppendToOutgoingContext(ctx, "traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

		require.NoError(t, call(ctx, conn, "Seeded"))

		got := <-seen
		span := oteltest.EndedNamed(t, recorder, "gst.test.Echo/Seeded")
		require.Equal(t, "0af7651916cd43dd8448eb211c80319c", got.traceID)
		require.Equal(t, "b7ad6b7169203331", span.Parent().SpanID().String())
	})

	t.Run("the caller's x-trace-id seeds the span when it sent no trace context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = metadata.AppendToOutgoingContext(ctx, "x-trace-id", "4bf92f3577b34da6a3ce929d0e0e4736")
		var header metadata.MD

		require.NoError(t, call(ctx, conn, "Quoted", grpc.Header(&header)))

		got := <-seen
		span := oteltest.EndedNamed(t, recorder, "gst.test.Echo/Quoted")
		require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", span.SpanContext().TraceID().String())
		require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", got.traceID)
		require.Equal(t, []string{"4bf92f3577b34da6a3ce929d0e0e4736"}, header.Get("x-trace-id"))
	})

	t.Run("a failed call ends the span with its status", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := call(ctx, conn, "Deny")

		require.Equal(t, codes.PermissionDenied, status.Code(err))
		span := oteltest.EndedNamed(t, recorder, "gst.test.Echo/Deny")
		// A refusal is the caller's error: the conventions leave the server
		// span's status unset for it and carry the code as an attribute; a
		// failure of the server's own, INTERNAL among them, sets the error
		// status (see the panic below).
		require.Equal(t, otelcodes.Unset, span.Status().Code)
		require.Equal(t, "PERMISSION_DENIED", attributesOf(span)["rpc.response.status_code"])
	})

	t.Run("a panic is recorded on the span", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := call(ctx, conn, "Boom")

		require.Equal(t, codes.Internal, status.Code(err))
		span := oteltest.EndedNamed(t, recorder, "gst.test.Echo/Boom")
		require.Equal(t, otelcodes.Error, span.Status().Code)
		attrs := attributesOf(span)
		require.Equal(t, "INTERNAL", attrs["rpc.response.status_code"])
		require.Equal(t, true, attrs["error.panic"])
		require.Equal(t, "sample panic", attrs["error.recovered"])
		names := make([]string, 0, len(span.Events()))
		for _, event := range span.Events() {
			names = append(names, event.Name)
		}
		require.Contains(t, names, "exception", "the panic is recorded as the span's exception event")
	})
}

// TestCallsRunWithoutASpanWithTracingOff pins that tracing off costs a call
// nothing: no span is opened, and the trace id is the one the request scope
// generates.
func TestCallsRunWithoutASpanWithTracingOff(t *testing.T) {
	reset(t)
	seen := make(chan traced, 1)
	serve(map[string]func(context.Context) error{"Look": func(ctx context.Context) error {
		seen <- traced{traceID: execctx.FromContext(ctx).TraceID, span: trace.SpanContextFromContext(ctx)}
		return nil
	}})
	addr := start(t)
	conn := dial(t, addr, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Look"))

	got := <-seen
	require.False(t, got.span.IsValid())
	require.NotEmpty(t, got.traceID)
}

// attributesOf maps a span's attributes by key.
func attributesOf(span sdktrace.ReadOnlySpan) map[attribute.Key]any {
	attrs := make(map[attribute.Key]any, len(span.Attributes()))
	for _, kv := range span.Attributes() {
		attrs[kv.Key] = kv.Value.AsInterface()
	}
	return attrs
}
