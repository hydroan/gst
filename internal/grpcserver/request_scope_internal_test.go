package grpcserver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/logfield"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
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
// request metadata — for a method the registration described with no
// action, the full method as route, path and request URI and POST as the
// method every gRPC call is on the wire (a method described with its action
// carries the action's, see
// TestCallsDescribedByAnActionCarryItsRouteAndMethod), the peer's address,
// the authority the call was addressed to, the user agent, no TLS on a
// plaintext listener — and the trace id, the caller's x-trace-id when it is
// a W3C trace id and a generated one otherwise (see requestctx.TraceIDOf,
// the rule the HTTP listener reads the header by too), published back in the
// response header either way.
func TestCallsCarryTheRequestMetadataAndTraceID(t *testing.T) {
	reset(t)
	seen := make(chan observed, 1)
	look(seen)
	addr := start(t)
	conn := dial(t, addr, nil)

	const callerTraceID = "0123456789abcdef0123456789abcdef"
	t.Run("with the caller's trace id", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = metadata.AppendToOutgoingContext(ctx, "x-trace-id", callerTraceID)
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
		require.True(t, got.meta.RequiresAuth(), "a method not described public requires auth")
		require.Equal(t, callerTraceID, got.identity.TraceID)
		require.Equal(t, []string{callerTraceID}, header.Get("x-trace-id"))
	})

	t.Run("with a trace id that is not one", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = metadata.AppendToOutgoingContext(ctx, "x-trace-id", "trace-1")
		var header metadata.MD

		require.NoError(t, call(ctx, conn, "Look", grpc.Header(&header)))

		got := <-seen
		require.Regexp(t, `^[0-9a-f]{32}$`, got.identity.TraceID, "a bare word is replaced by a new W3C trace id")
		require.Equal(t, []string{got.identity.TraceID}, header.Get("x-trace-id"))
	})

	t.Run("with a generated trace id", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var header metadata.MD

		require.NoError(t, call(ctx, conn, "Look", grpc.Header(&header)))

		got := <-seen
		require.Regexp(t, `^[0-9a-f]{32}$`, got.identity.TraceID)
		require.Equal(t, []string{got.identity.TraceID}, header.Get("x-trace-id"))
	})
}

// TestCallsDescribedByAnActionCarryItsRouteAndMethod pins the metadata of a
// call whose rpc the registration described with an HTTP action, the way
// the generated registration describes every rpc: the route and method are
// the action's, /api/records/:id and GET, what a hook, a log or a span
// reads whichever listener served the action; the path and request URI
// stay the full method, the call's target on the wire; and the access-log
// entry names the route and method the same way. The rpc of a Stream action,
// described with MethodStream, carries STREAM as its method.
func TestCallsDescribedByAnActionCarryItsRouteAndMethod(t *testing.T) {
	reset(t)
	seen := make(chan observed, 1)
	report := func(ctx context.Context) error {
		seen <- observed{meta: requestctx.FromContext(ctx), identity: execctx.FromContext(ctx)}
		return nil
	}
	serve(map[string]func(context.Context) error{"Look": report, "Tail": report},
		Method{Name: "/gst.test.Echo/Look", HTTPMethod: http.MethodGet, Route: "/api/records/:id"},
		Method{Name: "/gst.test.Echo/Tail", HTTPMethod: MethodStream, Route: "/api/records/:id/tail"})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Look"))

	got := <-seen
	require.Equal(t, "/api/records/:id", got.meta.Route())
	require.Equal(t, http.MethodGet, got.meta.Method())
	require.Equal(t, "/gst.test.Echo/Look", got.meta.Path())
	require.Equal(t, "/gst.test.Echo/Look", got.meta.RequestURI())
	entries := accessLog.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "/api/records/:id", fields[requestctx.KeyRoute])
	require.Equal(t, http.MethodGet, fields[requestctx.KeyMethod])
	require.Equal(t, "/gst.test.Echo/Look", fields[requestctx.KeyPath])

	require.NoError(t, call(ctx, conn, "Tail"))
	require.Equal(t, MethodStream, (<-seen).meta.Method())
	entries = accessLog.All()
	require.Len(t, entries, 2)
	require.Equal(t, MethodStream, entries[1].ContextMap()[requestctx.KeyMethod])
}

// TestCallsCarryWhetherTheirMethodRequiresAuth pins the RequiresAuth of the
// request metadata, the answer a ServiceContext built on the call gives: a
// method the registration described as public does not require auth, any
// other does, the way the routes outside the HTTP listener's public group
// do; nor does a call of the server's own services, the health and the
// reflection service, which the auth interceptors leave alone (see
// guardedCall), the way the HTTP listener's probes require none.
func TestCallsCarryWhetherTheirMethodRequiresAuth(t *testing.T) {
	reset(t)
	seen := make(chan bool, 1)
	report := func(ctx context.Context) error {
		seen <- requestctx.FromContext(ctx).RequiresAuth()
		return nil
	}
	serve(map[string]func(context.Context) error{"Look": report, "Open": report},
		Method{Name: "/gst.test.Echo/Look"}, Method{Name: "/gst.test.Echo/Open", Public: true})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Look"))
	require.True(t, <-seen)
	require.NoError(t, call(ctx, conn, "Open"))
	require.False(t, <-seen)

	health := enterCall(context.Background(), "/"+grpc_health_v1.Health_ServiceDesc.ServiceName+"/Check")
	require.False(t, health.meta.RequiresAuth(), "the health service is the server's own")
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
	const traceID = "1a2b3c4d5e6f70818293a4b5c6d7e8f9"
	ctx = metadata.AppendToOutgoingContext(ctx, "x-trace-id", traceID)

	require.NoError(t, call(ctx, conn, "Ping"))
	require.Error(t, call(ctx, conn, "Missing"))

	entries := accessLog.All()
	require.Len(t, entries, 2)
	require.Equal(t, "/gst.test.Echo/Ping", entries[0].Message)
	ok := entries[0].ContextMap()
	require.Equal(t, "OK", ok["grpc_code"])
	require.Equal(t, http.MethodPost, ok[requestctx.KeyMethod])
	require.Equal(t, "/gst.test.Echo/Ping", ok[requestctx.KeyRoute])
	require.Equal(t, "/gst.test.Echo/Ping", ok[requestctx.KeyPath])
	require.Contains(t, ok, consts.CTX_USERNAME)
	require.Contains(t, ok, consts.CTX_USER_ID)
	require.Contains(t, ok, consts.CTX_TENANT_ID)
	require.Equal(t, "127.0.0.1", ok["ip"])
	require.Contains(t, ok["user_agent"], "grpc-go/")
	require.Equal(t, traceID, ok[execctx.KeyTraceID])
	require.Contains(t, ok, logfield.KeyDuration)
	require.Contains(t, ok, logfield.KeyDurationHuman)
	require.NotContains(t, ok, "error")
	failed := entries[1].ContextMap()
	require.Equal(t, "NotFound", failed["grpc_code"])
	require.Equal(t, "no such record", failed["error"])
	require.Len(t, entries[1].Context, accessLogFieldCap, "the worst case must fill the capacity exactly: a new field bumps accessLogFieldCap")
}

// TestTheClientAddressForwardedByATrustedPeerIsTheClients pins that the
// scope believes the forwarding headers of the peers server.trusted_proxies
// names, the way the HTTP listener does: from such a peer the client address
// is the one X-Forwarded-For carries, in the metadata and in the access log
// alike, and from any other peer it is the peer itself, whatever it forwards.
func TestTheClientAddressForwardedByATrustedPeerIsTheClients(t *testing.T) {
	seen := make(chan string, 1)
	whoami := map[string]func(context.Context) error{
		"Whoami": func(ctx context.Context) error { seen <- requestctx.FromContext(ctx).ClientIP(); return nil },
	}
	forwarded := func(ctx context.Context) context.Context {
		return metadata.AppendToOutgoingContext(ctx, "x-forwarded-for", "203.0.113.9")
	}

	t.Run("from a trusted peer", func(t *testing.T) {
		reset(t)
		config.App.Server.TrustedProxies = []string{"127.0.0.1", "::1"}
		serve(whoami)
		conn := dial(t, start(t), nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		require.NoError(t, call(forwarded(ctx), conn, "Whoami"))
		require.Equal(t, "203.0.113.9", <-seen)
		require.Equal(t, "203.0.113.9", accessLog.All()[0].ContextMap()["ip"])
	})

	t.Run("from a trusted peer naming it in X-Real-IP alone", func(t *testing.T) {
		reset(t)
		config.App.Server.TrustedProxies = []string{"127.0.0.1", "::1"}
		serve(whoami)
		conn := dial(t, start(t), nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		require.NoError(t, call(metadata.AppendToOutgoingContext(ctx, "x-real-ip", "203.0.113.9"), conn, "Whoami"))
		require.Equal(t, "203.0.113.9", <-seen)
	})

	t.Run("from any other peer", func(t *testing.T) {
		reset(t)
		serve(whoami)
		conn := dial(t, start(t), nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		require.NoError(t, call(forwarded(ctx), conn, "Whoami"))
		require.Equal(t, "127.0.0.1", <-seen)
	})
}
