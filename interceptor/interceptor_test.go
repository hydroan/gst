package interceptor_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/hydroan/gst/authn/jwt"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/interceptor"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestRegisterAuthGuardsTheServicesThroughTheServer pins the entries end to
// end, on the server's own listener: the interceptors RegisterAuth adds run
// for every method of the registered services but the public ones, here
// JwtAuth, which lets a call carrying a token through, refuses one without,
// and is not consulted for a public method.
func TestRegisterAuthGuardsTheServicesThroughTheServer(t *testing.T) {
	withSigningKey(t, "test-signing-key")
	interceptor.RegisterAuth(interceptor.JwtAuth())
	grpcserver.Register(func(r grpc.ServiceRegistrar) {
		r.RegisterService(&grpc.ServiceDesc{
			ServiceName: "gst.test.Echo",
			HandlerType: (*any)(nil),
			Methods:     []grpc.MethodDesc{method("Ping"), method("Look")},
			Metadata:    "gst/test/echo.proto",
		}, nil)
	}, grpcserver.Method{Name: "/gst.test.Echo/Look", Public: true}, grpcserver.Method{Name: "/gst.test.Echo/Ping"})
	conn := dial(t, startServer(t))
	token, _, err := jwt.GenTokens("u-1", "alice")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	withToken := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	require.NoError(t, call(ctx, conn, "Look"), "a public method needs no token")
	require.Equal(t, codes.Unauthenticated, status.Code(call(ctx, conn, "Ping")))
	require.NoError(t, call(withToken, conn, "Ping"))
}

// method builds the descriptor of the unary rpc name of gst.test.Echo,
// answering an empty message with an empty message after running the
// server's interceptors, the way generated code does.
func method(name string) grpc.MethodDesc {
	return grpc.MethodDesc{
		MethodName: name,
		Handler: func(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
			in := new(emptypb.Empty)
			if err := dec(in); err != nil {
				return nil, err
			}
			handler := func(context.Context, any) (any, error) { return &emptypb.Empty{}, nil }
			if interceptor == nil {
				return handler(ctx, in)
			}
			return interceptor(ctx, in, &grpc.UnaryServerInfo{FullMethod: "/gst.test.Echo/" + name}, handler)
		},
	}
}

// startServer runs the server on a free port of the loopback interface and
// returns its address; the server stops when the test ends.
func startServer(t *testing.T) string {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr, ok := probe.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.NoError(t, probe.Close())
	saved := config.App.GRPC
	config.App.GRPC = config.GRPC{Listen: "127.0.0.1", Port: addr.Port}
	t.Cleanup(func() { config.App.GRPC = saved })
	errs := make(chan error, 1)
	go func() { errs <- grpcserver.Run() }()
	t.Cleanup(func() {
		grpcserver.Stop(context.Background())
		require.NoError(t, <-errs)
	})
	return addr.String()
}

// dial connects to addr in plaintext.
func dial(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// call invokes the rpc name of gst.test.Echo over conn, waiting for the
// connection to come up first.
func call(ctx context.Context, conn *grpc.ClientConn, name string) error {
	return conn.Invoke(ctx, "/gst.test.Echo/"+name, &emptypb.Empty{}, &emptypb.Empty{}, grpc.WaitForReady(true))
}
