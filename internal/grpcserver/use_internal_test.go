package grpcserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/consts"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestUseRunsTheProjectInterceptorsInsideTheBuiltinChain pins where the
// interceptors a project registers run: inside the framework's own chain,
// the common ones before the auth ones and each group in registration
// order, and around the handler.
func TestUseRunsTheProjectInterceptorsInsideTheBuiltinChain(t *testing.T) {
	reset(t)
	var mu sync.Mutex
	var order []string
	note := func(name string) grpc.UnaryServerInterceptor {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return handler(ctx, req)
		}
	}
	UseAuth(note("auth"))
	Use(note("common-1"), note("common-2"))
	serve(map[string]func(context.Context) error{"Ping": func(context.Context) error {
		mu.Lock()
		order = append(order, "handler")
		mu.Unlock()
		return nil
	}})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Ping"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"common-1", "common-2", "auth", "handler"}, order)
}

// TestUseAuthSkipsThePublicMethods pins that the auth interceptors leave the
// methods a service registers as public alone and guard every other one,
// the way the HTTP listener keeps Public() routes out of the authenticated
// group.
func TestUseAuthSkipsThePublicMethods(t *testing.T) {
	reset(t)
	UseAuth(func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) {
		return nil, status.Error(codes.Unauthenticated, "no credentials")
	})
	serve(map[string]func(context.Context) error{
		"Ping": func(context.Context) error { return nil },
		"Look": func(context.Context) error { return nil },
	}, Method{Name: "/gst.test.Echo/Look", Public: true}, Method{Name: "/gst.test.Echo/Ping"})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Look"), "a public method needs no credentials")
	require.Equal(t, codes.Unauthenticated, status.Code(call(ctx, conn, "Ping")))
}

// TestWithCallerNamesTheCallerDownstreamAndInTheAccessLog pins what an
// auth interceptor establishing the caller hands on: the request metadata
// the handler reads carries the caller beside everything it carried
// before, and so does the call's access-log entry.
func TestWithCallerNamesTheCallerDownstreamAndInTheAccessLog(t *testing.T) {
	reset(t)
	UseAuth(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(WithCaller(ctx, Caller{Username: "alice", UserID: "u-1", SessionID: "s-1", TenantID: "t-1"}), req)
	})
	seen := make(chan observed, 1)
	look(seen)
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Look"))

	got := <-seen
	require.Equal(t, "alice", got.meta.Username())
	require.Equal(t, "u-1", got.meta.UserID())
	require.Equal(t, "s-1", got.meta.SessionID())
	require.Equal(t, "t-1", got.meta.TenantID())
	require.Equal(t, "/gst.test.Echo/Look", got.meta.Route(), "the rest of the metadata stays")
	require.Equal(t, "127.0.0.1", got.meta.ClientIP())
	entries := accessLog.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "alice", fields[consts.CTX_USERNAME])
	require.Equal(t, "u-1", fields[consts.CTX_USER_ID])
}

// TestAPanicInAProjectInterceptorIsRecovered pins that the framework's
// recovery wraps the project's interceptors as it wraps the handler: the
// caller gets codes.Internal, the panic is logged and the server goes on.
func TestAPanicInAProjectInterceptorIsRecovered(t *testing.T) {
	reset(t)
	Use(func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) { panic("boom") })
	echo(nil, nil)
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := call(ctx, conn, "Ping")

	require.Equal(t, codes.Internal, status.Code(err))
	require.Len(t, recoveryLog.All(), 1)
	require.Equal(t, codes.Internal, status.Code(call(ctx, conn, "Ping")), "the server must outlive the panic")
}

// TestUseAfterRunPanics pins that an interceptor registered once the server
// runs is a programming error, reported at once rather than left out of the
// chain quietly, like a service registered late.
func TestUseAfterRunPanics(t *testing.T) {
	reset(t)
	echo(nil, nil)
	start(t)
	pass := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(ctx, req)
	}

	require.Panics(t, func() { Use(pass) })
	require.Panics(t, func() { UseAuth(pass) })
}

// TestRouteAndCallerOfDescribeTheCall pins what the interceptors of the
// modules read off a call: Route answers the HTTP method and route the
// registration described the rpc with, the ones the same action is served
// at over HTTP, and CallerOf answers the caller an earlier interceptor
// established; outside a call both answer nothing.
func TestRouteAndCallerOfDescribeTheCall(t *testing.T) {
	reset(t)
	type seen struct {
		httpMethod, route string
		caller            Caller
	}
	got := make(chan seen, 1)
	UseAuth(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = WithCaller(ctx, Caller{Username: "alice", UserID: "u-1"})
		httpMethod, route := Route(ctx)
		got <- seen{httpMethod: httpMethod, route: route, caller: CallerOf(ctx)}
		return handler(ctx, req)
	})
	serve(map[string]func(context.Context) error{"Ping": func(context.Context) error { return nil }},
		Method{Name: "/gst.test.Echo/Ping", HTTPMethod: "GET", Route: "/api/records"})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Ping"))

	require.Equal(t, seen{httpMethod: "GET", route: "/api/records", caller: Caller{Username: "alice", UserID: "u-1"}}, <-got)
	httpMethod, route := Route(context.Background())
	require.Empty(t, httpMethod)
	require.Empty(t, route)
	require.Equal(t, Caller{}, CallerOf(context.Background()))
}
