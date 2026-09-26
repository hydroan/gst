package authz_test

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/tenant"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestAuthzInterceptor pins the gRPC counterpart of the authorization
// middleware on a real listener, behind the session interceptor the way the
// middleware runs behind IAMSession: a call is judged by the HTTP method and
// route its action is served at, so the policy set is one for both
// listeners. A call without a session is refused before any decision, a
// subject holding no role is refused with the middleware's message, a role
// carrying the permission reaches the action and nothing else, and the root
// subject reaches everything.
func TestAuthzInterceptor(t *testing.T) {
	conn := grpcAuthzProbe(t)
	// The sessions are established presenting the user agent grpc-go
	// presents, the device the session interceptor holds a call to.
	userID, sessionID := authzSignupAndLoginUserWithUserAgent(t, authzTestUsername("grpc_authorization"), "12345678", grpcUserAgent)
	subject := authorizationSubject{userID: userID, sessionID: sessionID}

	t.Run("without a session", func(t *testing.T) {
		require.Equal(t, codes.Unauthenticated, status.Code(authzProbeCall(t, conn, "Look", "")))
	})

	t.Run("holding no role", func(t *testing.T) {
		err := authzProbeCall(t, conn, "Look", subject.sessionID)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Equal(t, "permission denied", status.Convert(err).Message())
	})

	t.Run("holding a role with the permission", func(t *testing.T) {
		roleID := newAuthorizationRole(t, subject, "grpc_authorization")
		authzGrantTenantPolicy(t, tenant.Default, roleID, types.Permission{Object: grantedRoute, Action: http.MethodGet})

		require.NoError(t, authzProbeCall(t, conn, "Look", subject.sessionID))
		require.Equal(t, subject.userID, authzProbeLastCaller(t).UserID)
		require.Equal(t, codes.PermissionDenied, status.Code(authzProbeCall(t, conn, "Deny", subject.sessionID)), "the role carries no permission for the other action")
	})

	t.Run("as root", func(t *testing.T) {
		rootSessionID := loginSessionIDFromCookieWithUserAgent(t, iam.LoginReq{Username: rootUsername, Password: rootPassword}, grpcUserAgent)
		require.NoError(t, authzProbeCall(t, conn, "Deny", rootSessionID))
		require.Equal(t, consts.AUTHZ_USER_ROOT, authzProbeLastCaller(t).UserID)
	})
}

// The probe service stands for a project's gRPC service behind the module's
// interceptors: gst.test.Probe with Look served at GET grantedRoute and Deny
// at GET deniedRoute, the two routes the HTTP cases grant and refuse. One
// listener serves the whole test binary, the server's registrations being
// made once.
var (
	grpcAuthzProbeOnce   sync.Once
	grpcAuthzProbeAddr   string
	errGRPCAuthzProbe    error
	grpcAuthzProbeMu     sync.Mutex
	grpcAuthzProbeCaller gstgrpc.Caller
)

// grpcUserAgent is the user agent grpc-go presents for the probe's calls: a
// session a call names has to have been established presenting it.
const grpcUserAgent = "grpc-go/" + grpc.Version

// grpcAuthzProbe returns a connection to the probe listener, starting it on
// first use behind the session and authorization interceptors iam.Register
// and authz.Register mounted.
func grpcAuthzProbe(t *testing.T) *grpc.ClientConn {
	t.Helper()
	grpcAuthzProbeOnce.Do(func() {
		handle := func(name string) grpc.MethodDesc {
			return grpc.MethodDesc{
				MethodName: name,
				Handler: func(_ any, ctx context.Context, dec func(any) error, unary grpc.UnaryServerInterceptor) (any, error) {
					in := new(emptypb.Empty)
					if err := dec(in); err != nil {
						return nil, err
					}
					handler := func(ctx context.Context, _ any) (any, error) {
						grpcAuthzProbeMu.Lock()
						grpcAuthzProbeCaller = gstgrpc.CallerOf(ctx)
						grpcAuthzProbeMu.Unlock()
						return &emptypb.Empty{}, nil
					}
					return unary(ctx, in, &grpc.UnaryServerInfo{FullMethod: "/gst.test.Probe/" + name}, handler)
				},
			}
		}
		grpcserver.Register(func(r grpc.ServiceRegistrar) {
			r.RegisterService(&grpc.ServiceDesc{
				ServiceName: "gst.test.Probe",
				HandlerType: (*any)(nil),
				Methods:     []grpc.MethodDesc{handle("Look"), handle("Deny")},
				Metadata:    "gst/test/probe.proto",
			}, nil)
		},
			grpcserver.Method{Name: "/gst.test.Probe/Look", HTTPMethod: http.MethodGet, Route: grantedRoute},
			grpcserver.Method{Name: "/gst.test.Probe/Deny", HTTPMethod: http.MethodGet, Route: deniedRoute},
		)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			errGRPCAuthzProbe = err
			return
		}
		addr, _ := listener.Addr().(*net.TCPAddr)
		_ = listener.Close()
		config.App.GRPC = config.GRPC{Listen: "127.0.0.1", Port: addr.Port}
		grpcAuthzProbeAddr = addr.String()
		go func() { _ = grpcserver.Run() }()
	})
	require.NoError(t, errGRPCAuthzProbe)
	conn, err := grpc.NewClient(grpcAuthzProbeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// authzProbeCall invokes the probe rpc name, naming sessionID as the call's
// session when it is not empty.
func authzProbeCall(t *testing.T, conn *grpc.ClientConn, name, sessionID string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if sessionID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+sessionID)
	}
	return conn.Invoke(ctx, "/gst.test.Probe/"+name, &emptypb.Empty{}, &emptypb.Empty{}, grpc.WaitForReady(true))
}

// authzProbeLastCaller returns the caller the probe handler saw on its last
// call.
func authzProbeLastCaller(t *testing.T) gstgrpc.Caller {
	t.Helper()
	grpcAuthzProbeMu.Lock()
	defer grpcAuthzProbeMu.Unlock()
	return grpcAuthzProbeCaller
}
