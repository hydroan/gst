package grpc_test

import (
	"context"
	"testing"

	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

// TestBearerReadsTheCredentialOfTheCall pins what Bearer answers: the
// credential of an "authorization: Bearer <credential>" metadata, and no
// credential for another scheme or no authorization at all.
func TestBearerReadsTheCredentialOfTheCall(t *testing.T) {
	credential, ok := gstgrpc.Bearer(metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer s-1")))
	require.True(t, ok)
	require.Equal(t, "s-1", credential)

	_, ok = gstgrpc.Bearer(metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Basic YWxpY2U6cGFzcw==")))
	require.False(t, ok)
	_, ok = gstgrpc.Bearer(context.Background())
	require.False(t, ok)
}

// TestWithCallerNamesTheCallerForTheContext pins the pair an interceptor
// uses: WithCaller puts the caller on the request metadata the service
// context reads, and CallerOf reads it back, the zero Caller outside a call
// or before one is established.
func TestWithCallerNamesTheCallerForTheContext(t *testing.T) {
	require.Equal(t, gstgrpc.Caller{}, gstgrpc.CallerOf(context.Background()))

	ctx := gstgrpc.WithCaller(context.Background(), gstgrpc.Caller{Username: "alice", UserID: "u-1", SessionID: "s-1", TenantID: "t-1"})

	meta := requestctx.FromContext(ctx)
	require.Equal(t, "alice", meta.Username())
	require.Equal(t, "u-1", meta.UserID())
	require.Equal(t, "s-1", meta.SessionID())
	require.Equal(t, "t-1", meta.TenantID())
}

// TestRouteIsEmptyOutsideACall pins that Route answers nothing for a
// context no call runs on.
func TestRouteIsEmptyOutsideACall(t *testing.T) {
	httpMethod, route := gstgrpc.Route(context.Background())
	require.Empty(t, httpMethod)
	require.Empty(t, route)
}
