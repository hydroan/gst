package grpcserver

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/stretchr/testify/require"
)

// TestWithCallerNamesTheCallerDownstreamAndInTheAccessLog pins what an
// auth interceptor establishing the caller hands on: the request metadata
// the handler reads carries the caller beside everything it carried
// before, and so does the call's access-log entry.
func TestWithCallerNamesTheCallerDownstreamAndInTheAccessLog(t *testing.T) {
	reset(t)
	UseAuth(func(ctx context.Context) (context.Context, error) {
		return WithCaller(ctx, Caller{Username: "alice", UserID: "u-1", SessionID: "s-1", TenantID: "t-1"}), nil
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
	UseAuth(func(ctx context.Context) (context.Context, error) {
		ctx = WithCaller(ctx, Caller{Username: "alice", UserID: "u-1"})
		httpMethod, route := Route(ctx)
		got <- seen{httpMethod: httpMethod, route: route, caller: CallerOf(ctx)}
		return ctx, nil
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

// TestWithParamsAttachesTheParametersOfTheCall pins what the call functions
// of the controller attach to a call: the route parameters and the query the
// request message carried, answered by the request metadata the way a
// request's are, beside what the call already carried, the method as route
// and the caller an interceptor established; outside a call the metadata
// answers the parameters alone.
func TestWithParamsAttachesTheParametersOfTheCall(t *testing.T) {
	reset(t)
	got := make(chan requestctx.Metadata, 1)
	UseAuth(func(ctx context.Context) (context.Context, error) {
		return WithCaller(ctx, Caller{Username: "alice", UserID: "u-1"}), nil
	})
	serve(map[string]func(context.Context) error{"Ping": func(ctx context.Context) error {
		ctx = WithParams(ctx, map[string]string{"box": "b-1"}, url.Values{"_page": {"2"}})
		got <- requestctx.FromContext(ctx)
		return nil
	}}, Method{Name: "/gst.test.Echo/Ping"})
	conn := dial(t, start(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, call(ctx, conn, "Ping"))

	meta := <-got
	require.Equal(t, "b-1", meta.Param("box"))
	require.Equal(t, url.Values{"_page": {"2"}}, meta.Query())
	require.Equal(t, "/gst.test.Echo/Ping", meta.Route())
	require.Equal(t, "alice", meta.Username())
	require.Equal(t, "u-1", meta.UserID())
	require.True(t, meta.RequiresAuth())

	outside := requestctx.FromContext(WithParams(context.Background(), map[string]string{"box": "b-2"}, nil))
	require.Equal(t, "b-2", outside.Param("box"))
	require.Empty(t, outside.Username())
}
