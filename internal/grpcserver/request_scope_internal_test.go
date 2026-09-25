package grpcserver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
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

// TestCallsAreLoggedLikeHTTPRequests pins the access log entry of a call:
// one entry per call in logger.GRPC, with the fields the HTTP access log
// carries — the status as the code's name, the method, the route and the
// path, the caller's identity, the peer address, the user agent, the trace
// id and the duration — and, for a failed call, the status message, the
// worst case, which fills accessLogFieldCap exactly: a field added without
// bumping the capacity fails here instead of regrowing the slice on every
// call.
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
	require.Len(t, entries[1].Context, accessLogFieldCap, "the worst case must fill the capacity exactly: a new field bumps accessLogFieldCap")
}
