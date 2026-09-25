package grpcserver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// observed is what a call's handler found on its context.
type observed struct {
	meta     requestctx.Metadata
	identity execctx.Identity
}

// look registers the rpc Look, whose handler reports what it finds on its
// context to seen and answers an empty message.
func look(seen chan<- observed) {
	serve(map[string]func(context.Context) error{"Look": func(ctx context.Context) error {
		seen <- observed{meta: requestctx.FromContext(ctx), identity: execctx.FromContext(ctx)}
		return nil
	}})
}

// TestCallsCarryTheRequestMetadataAndTraceID pins what a handler finds on
// its context, the facts an HTTP handler finds on its request context: the
// request metadata — the full method as route, path and request URI, POST
// as the method every gRPC call is on the wire, the peer's address, the
// authority the call was addressed to, the user agent, no TLS on a
// plaintext listener — and the trace id, the caller's x-trace-id when it
// sent one and a generated one otherwise, published back in the response
// header either way.
func TestCallsCarryTheRequestMetadataAndTraceID(t *testing.T) {
	reset(t)
	seen := make(chan observed, 1)
	look(seen)
	addr := start(t)
	conn := dial(t, addr, nil)

	t.Run("with the caller's trace id", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = metadata.AppendToOutgoingContext(ctx, "x-trace-id", "trace-1")
		var header metadata.MD

		require.NoError(t, call(ctx, conn, "Look", grpc.Header(&header)))

		got := <-seen
		require.Equal(t, "/gst.test.Echo/Look", got.meta.Route())
		require.Equal(t, "/gst.test.Echo/Look", got.meta.Path())
		require.Equal(t, "/gst.test.Echo/Look", got.meta.RequestURI())
		require.Equal(t, http.MethodPost, got.meta.Method())
		require.Equal(t, "127.0.0.1", got.meta.ClientIP())
		require.Equal(t, addr, got.meta.Host())
		require.Contains(t, got.meta.UserAgent(), "grpc-go/")
		require.False(t, got.meta.TLS())
		require.Equal(t, "trace-1", got.identity.TraceID)
		require.Equal(t, []string{"trace-1"}, header.Get("x-trace-id"))
	})

	t.Run("with a generated trace id", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var header metadata.MD

		require.NoError(t, call(ctx, conn, "Look", grpc.Header(&header)))

		got := <-seen
		require.NotEmpty(t, got.identity.TraceID)
		require.Equal(t, []string{got.identity.TraceID}, header.Get("x-trace-id"))
	})
}

// TestAPanicAnswersInternalAndIsLoggedWithItsStack pins the bounds of a
// panic in a handler: the caller gets codes.Internal with the message the
// HTTP envelope carries for the same case, the server goes on answering,
// and the recovery log, the one the HTTP listener's panics go to, has the
// panic, the method, the trace id and the stack.
func TestAPanicAnswersInternalAndIsLoggedWithItsStack(t *testing.T) {
	reset(t)
	serve(map[string]func(context.Context) error{
		"Boom": func(context.Context) error { panic("boom") },
		"Ping": func(context.Context) error { return nil },
	})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-trace-id", "trace-panic")
	var header metadata.MD

	err := call(ctx, conn, "Boom", grpc.Header(&header))

	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, "internal server error", status.Convert(err).Message())
	require.Equal(t, []string{"trace-panic"}, header.Get("x-trace-id"), "the caller quotes the trace id back from the header of the failed call")
	require.NoError(t, call(ctx, conn, "Ping"), "the server must outlive the panic")
	entries := recoveryLog.All()
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Message, "panic recovered")
	require.Contains(t, entries[0].Message, "boom")
	require.Contains(t, entries[0].Message, "/gst.test.Echo/Boom")
	require.Contains(t, entries[0].Message, "goroutine ", "the stack of the panic")
	require.Equal(t, "trace-panic", entries[0].ContextMap()[consts.TRACE_ID])
}

// TestCallsAreLoggedLikeHTTPRequests pins the access log entry of a call:
// one entry per call in logger.GRPC, with the fields the HTTP access log
// carries — the status as the code's name, the method, the route and the
// path, the caller's identity, the peer address, the user agent, the trace
// id and the duration — and, for a failed call, the status message.
func TestCallsAreLoggedLikeHTTPRequests(t *testing.T) {
	reset(t)
	serve(map[string]func(context.Context) error{
		"Ping":    func(context.Context) error { return nil },
		"Missing": func(context.Context) error { return status.Error(codes.NotFound, "no such record") },
	})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-trace-id", "trace-log")

	require.NoError(t, call(ctx, conn, "Ping"))
	require.Error(t, call(ctx, conn, "Missing"))

	entries := accessLog.All()
	require.Len(t, entries, 2)
	require.Equal(t, "/gst.test.Echo/Ping", entries[0].Message)
	ok := entries[0].ContextMap()
	require.Equal(t, "OK", ok["status"])
	require.Equal(t, http.MethodPost, ok[consts.CTX_METHOD])
	require.Equal(t, "/gst.test.Echo/Ping", ok[consts.CTX_ROUTE])
	require.Equal(t, "/gst.test.Echo/Ping", ok[consts.CTX_PATH])
	require.Contains(t, ok, consts.CTX_USERNAME)
	require.Contains(t, ok, consts.CTX_USER_ID)
	require.Equal(t, "127.0.0.1", ok["ip"])
	require.Contains(t, ok["user_agent"], "grpc-go/")
	require.Equal(t, "trace-log", ok[consts.TRACE_ID])
	require.Contains(t, ok, consts.LOG_DURATION)
	require.Contains(t, ok, consts.LOG_DURATION_HUMAN)
	require.NotContains(t, ok, "error")
	failed := entries[1].ContextMap()
	require.Equal(t, "NotFound", failed["status"])
	require.Equal(t, "no such record", failed["error"])
}

// TestCallsAreCountedInTheMetrics pins that the calls show up in the
// standard grpc_server_* metrics on the default registry, the one the
// metrics endpoint serves: the handled counter, labeled with the service,
// the method, the call type and the code, and the handling-time histogram.
func TestCallsAreCountedInTheMetrics(t *testing.T) {
	reset(t)
	echo(nil, nil)
	conn := dial(t, start(t), nil)
	handled := func() int {
		labels := map[string]string{"grpc_service": "gst.test.Echo", "grpc_method": "Ping", "grpc_type": "unary", "grpc_code": "OK"}
		return int(metricSample(t, "grpc_server_handled_total", labels).GetCounter().GetValue())
	}
	timed := func() uint64 {
		labels := map[string]string{"grpc_service": "gst.test.Echo", "grpc_method": "Ping", "grpc_type": "unary"}
		return metricSample(t, "grpc_server_handling_seconds", labels).GetHistogram().GetSampleCount()
	}
	handledBefore, timedBefore := handled(), timed()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Ping"))

	require.Equal(t, handledBefore+1, handled())
	require.Equal(t, timedBefore+1, timed())
}

// metricSample reads the sample of family carrying labels off the default
// registry, nil when there is none: the getters of a nil sample answer zero.
func metricSample(t *testing.T, family string, labels map[string]string) *dto.Metric {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != family {
			continue
		}
	samples:
		for _, m := range f.GetMetric() {
			for _, pair := range m.GetLabel() {
				if want, ok := labels[pair.GetName()]; ok && want != pair.GetValue() {
					continue samples
				}
			}
			return m
		}
	}
	return nil
}
