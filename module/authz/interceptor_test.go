package authz_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/grpcserver"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/internal/modelregistry"
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
// the route template of its action, the route with every parameter written
// {name} the way the route list spells it, so a policy written for the
// route list decides for both listeners. A call without a session is
// refused before any decision, a subject holding no role is refused with
// the middleware's message, a role carrying the permission reaches the
// action and nothing else, and the root subject reaches everything. On a
// route with a parameter, a policy naming the template grants the call, one
// spelling the parameter the gin way, :thing, matches nothing, and one
// naming a concrete path grants an HTTP request alone. A Stream action is
// granted by the STREAM word on the template of its route, an HTTP method
// granting it nothing.
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

	t.Run("on a route with a parameter", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			user   string
			object string
			code   codes.Code
		}{
			{name: "the template grants the call", user: "grpc_peek_template", object: "/api/probe-things/{thing}", code: codes.OK},
			{name: "the gin spelling of the parameter matches nothing", user: "grpc_peek_gin", object: "/api/probe-things/:thing", code: codes.PermissionDenied},
			{name: "a concrete path grants an HTTP request alone", user: "grpc_peek_concrete", object: "/api/probe-things/42", code: codes.PermissionDenied},
		} {
			t.Run(tt.name, func(t *testing.T) {
				peeker := authorizationSubject{}
				peeker.userID, peeker.sessionID = authzSignupAndLoginUserWithUserAgent(t, authzTestUsername(tt.user), "12345678", grpcUserAgent)
				roleID := newAuthorizationRole(t, peeker, tt.user)
				authzGrantTenantPolicy(t, tenant.Default, roleID, types.Permission{Object: tt.object, Action: http.MethodGet})

				require.Equal(t, tt.code, status.Code(authzProbeCall(t, conn, "Peek", peeker.sessionID)))
			})
		}
	})

	t.Run("on a stream action", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			user   string
			action string
			code   codes.Code
		}{
			{name: "the STREAM word grants the stream", user: "grpc_tail_stream", action: gstgrpc.MethodStream, code: codes.OK},
			{name: "an HTTP method grants nothing", user: "grpc_tail_get", action: http.MethodGet, code: codes.PermissionDenied},
		} {
			t.Run(tt.name, func(t *testing.T) {
				tailer := authorizationSubject{}
				tailer.userID, tailer.sessionID = authzSignupAndLoginUserWithUserAgent(t, authzTestUsername(tt.user), "12345678", grpcUserAgent)
				roleID := newAuthorizationRole(t, tailer, tt.user)
				authzGrantTenantPolicy(t, tenant.Default, roleID, types.Permission{Object: "/api/probe-things/{thing}/tail", Action: tt.action})

				require.Equal(t, tt.code, status.Code(authzProbeStream(t, conn, tailer.sessionID)))
			})
		}
	})
}

// TestTenantAdminManagesUsersOverGRPC pins that iam's tenant administration
// judges a gRPC call by the route of the action, the way it judges an HTTP
// request by its path: a user a policy grants the user list on lists the
// users over the probe's ListUsers, which runs iam's user list the way a
// generated handler does, and a user without the grant is refused.
func TestTenantAdminManagesUsersOverGRPC(t *testing.T) {
	conn := grpcAuthzProbe(t)
	admin := authorizationSubject{}
	admin.userID, admin.sessionID = authzSignupAndLoginUserWithUserAgent(t, authzTestUsername("grpc_tenant_admin"), "12345678", grpcUserAgent)
	roleID := newAuthorizationRole(t, admin, "grpc_tenant_admin")
	authzGrantTenantPolicy(t, tenant.Default, roleID, types.Permission{Object: userAdminPath, Action: http.MethodGet})

	require.Equal(t, codes.OK, status.Code(authzProbeCall(t, conn, "ListUsers", admin.sessionID)))

	_, plainSessionID := authzSignupAndLoginUserWithUserAgent(t, authzTestUsername("grpc_plain_user"), "12345678", grpcUserAgent)
	require.Equal(t, codes.PermissionDenied, status.Code(authzProbeCall(t, conn, "ListUsers", plainSessionID)))
}

// The probe service stands for a project's gRPC service behind the module's
// interceptors: gst.test.Probe with Look served at GET grantedRoute and Deny
// at GET deniedRoute, the two routes the HTTP cases grant and refuse, Peek
// at GET probeThingRoute, a route with a parameter, Tail, a Stream action
// at probeTailRoute, and ListUsers, which runs iam's user list at GET
// userAdminPath the way a generated handler runs a service. One listener
// serves the whole test binary, the server's registrations being made
// once.
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

// probeThingRoute is the route with a parameter the probe's Peek is served
// at, as a registration writes it; probeTailRoute is the route of the
// probe's Tail, a Stream action, which a policy grants by the STREAM word on
// the route's template.
const (
	probeThingRoute = "/api/probe-things/:thing"
	probeTailRoute  = "/api/probe-things/:thing/tail"
)

// grpcAuthzProbe returns a connection to the probe listener, starting it on
// first use behind the session and authorization interceptors TestMain
// mounted.
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
		listUsers := gstgrpc.ServiceCall[*modeliamuser.User, *modelregistry.Empty, *modeliamuser.AdminUserListRsp](consts.List, userAdminPath)
		users := grpc.MethodDesc{
			MethodName: "ListUsers",
			Handler: func(_ any, ctx context.Context, dec func(any) error, unary grpc.UnaryServerInterceptor) (any, error) {
				in := new(emptypb.Empty)
				if err := dec(in); err != nil {
					return nil, err
				}
				handler := func(ctx context.Context, _ any) (any, error) {
					if _, err := listUsers(ctx, nil, gstgrpc.Query{}, new(modelregistry.Empty)); err != nil {
						return nil, err
					}
					return &emptypb.Empty{}, nil
				}
				return unary(ctx, in, &grpc.UnaryServerInfo{FullMethod: "/gst.test.Probe/ListUsers"}, handler)
			},
		}
		grpcserver.Register(func(r grpc.ServiceRegistrar) {
			r.RegisterService(&grpc.ServiceDesc{
				ServiceName: "gst.test.Probe",
				HandlerType: (*any)(nil),
				Methods:     []grpc.MethodDesc{handle("Look"), handle("Deny"), handle("Peek"), users},
				Streams: []grpc.StreamDesc{{
					StreamName:    "Tail",
					ServerStreams: true,
					Handler: func(_ any, ss grpc.ServerStream) error {
						if err := ss.RecvMsg(new(emptypb.Empty)); err != nil {
							return err
						}
						grpcAuthzProbeMu.Lock()
						grpcAuthzProbeCaller = gstgrpc.CallerOf(ss.Context())
						grpcAuthzProbeMu.Unlock()
						return ss.SendMsg(&emptypb.Empty{})
					},
				}},
				Metadata: "gst/test/probe.proto",
			}, nil)
		},
			grpcserver.Method{Name: "/gst.test.Probe/Look", HTTPMethod: http.MethodGet, Route: grantedRoute},
			grpcserver.Method{Name: "/gst.test.Probe/Deny", HTTPMethod: http.MethodGet, Route: deniedRoute},
			grpcserver.Method{Name: "/gst.test.Probe/Peek", HTTPMethod: http.MethodGet, Route: probeThingRoute},
			grpcserver.Method{Name: "/gst.test.Probe/Tail", HTTPMethod: grpcserver.MethodStream, Route: probeTailRoute},
			grpcserver.Method{Name: "/gst.test.Probe/ListUsers", HTTPMethod: http.MethodGet, Route: userAdminPath},
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

// authzProbeStream opens the probe's Tail stream naming sessionID as the
// call's session, sends its one request and returns the error the answer
// ends with, the refusal of a stream arriving with its first response.
func authzProbeStream(t *testing.T, conn *grpc.ClientConn, sessionID string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+sessionID)
	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{StreamName: "Tail", ServerStreams: true}, "/gst.test.Probe/Tail", grpc.WaitForReady(true))
	if err != nil {
		return err
	}
	// A refused stream may already be over by the time the request goes
	// out, which SendMsg reports as io.EOF; the status is read back.
	if err := stream.SendMsg(&emptypb.Empty{}); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}
	return stream.RecvMsg(&emptypb.Empty{})
}

// authzProbeLastCaller returns the caller the probe handler saw on its last
// call.
func authzProbeLastCaller(t *testing.T) gstgrpc.Caller {
	t.Helper()
	grpcAuthzProbeMu.Lock()
	defer grpcAuthzProbeMu.Unlock()
	return grpcAuthzProbeCaller
}
