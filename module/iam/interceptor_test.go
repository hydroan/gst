package iam_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/grpcserver"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	"github.com/hydroan/gst/internal/modelregistry"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TestIAMSessionInterceptor pins the gRPC counterpart of the session
// middleware on a real listener: a call names its session as
// "authorization: Bearer <session id>", and the interceptor admits a live
// session established presenting the user agent the client calls with, with
// the session's user as the caller, refuses a missing or unknown one and one
// established from another client with the fixed messages the middleware
// answers, answers a session the store cannot read as the server's own
// failure, Internal, leaving the session for the store to come back, leaves
// a public method alone, and, while the session requires a password change,
// admits only the actions a user needs to change it.
func TestIAMSessionInterceptor(t *testing.T) {
	conn := grpcProbe(t)
	account := newSessionTestAccount(t)
	sessionID := loginSession(t, account.Username, account.Password, client.WithUserAgent(grpcUserAgent))
	t.Cleanup(func() {
		require.NoError(t, serviceiamsession.Store.DeleteUserSessions(context.Background(), account.UserID))
	})

	t.Run("without a session", func(t *testing.T) {
		err := probeCall(t, conn, "Look", "")
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		require.Equal(t, "no session", status.Convert(err).Message())
	})

	t.Run("with an unknown session", func(t *testing.T) {
		err := probeCall(t, conn, "Look", strings.Repeat("0", 64))
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		require.Equal(t, "session invalid", status.Convert(err).Message())
	})

	t.Run("with a session established from another client", func(t *testing.T) {
		// A session is bound to the device it was established from over
		// either listener, so a browser's session is no good to a gRPC
		// client, the way it is no good to another browser.
		elsewhere := loginSession(t, account.Username, account.Password, client.WithUserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"))
		err := probeCall(t, conn, "Look", elsewhere)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		require.Equal(t, "session invalid", status.Convert(err).Message())
	})

	t.Run("with a live session", func(t *testing.T) {
		require.NoError(t, probeCall(t, conn, "Look", sessionID))
		session := loadStoredSession(t, sessionID)
		require.Equal(t, gstgrpc.Caller{UserID: account.UserID, Username: account.Username, SessionID: sessionID, TenantID: session.TenantID}, probeLastCaller(t))
	})

	t.Run("with a session the store cannot read", func(t *testing.T) {
		restore := corruptSessionSnapshot(t, sessionID)

		err := probeCall(t, conn, "Look", sessionID)
		require.Equal(t, codes.Internal, status.Code(err))
		require.Equal(t, "failed to load session", status.Convert(err).Message())

		restore()
		require.NoError(t, probeCall(t, conn, "Look", sessionID), "the session serves again once the store reads it")
	})

	t.Run("on a public method", func(t *testing.T) {
		require.NoError(t, probeCall(t, conn, "Open", ""))
	})

	t.Run("while a password change is required", func(t *testing.T) {
		// The flag is the password credential's, refreshed into the session
		// through the user-state cache, which is dropped so the next call
		// reads the credential again.
		credential := accountRequirePasswordCredential(t, account.UserID)
		credential.MustChangePassword = true
		require.NoError(t, database.Database[*modeliamaccount.PasswordCredential](context.Background()).
			WithoutHook().
			WithSelect(colCredentialUserID, colMustChangePassword).
			Update(credential))
		serviceiamsession.Store.DropUserState(t.Context(), account.UserID)

		err := probeCall(t, conn, "Look", sessionID)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Equal(t, "password change required before using this resource", status.Convert(err).Message())
		require.NoError(t, probeCall(t, conn, "ChangePassword", sessionID), "the action that changes the password stays reachable")
	})
}

// TestCurrentSessionOverGRPC pins that an action reading the current session
// serves a gRPC call: the session the interceptor admitted is read off the
// context with no cookie asked for, so the call, run through the framework's
// call function and its check of the HTTP-only methods, answers with the
// session the call named.
func TestCurrentSessionOverGRPC(t *testing.T) {
	conn := grpcProbe(t)
	account := newSessionTestAccount(t)
	sessionID := loginSession(t, account.Username, account.Password, client.WithUserAgent(grpcUserAgent))
	t.Cleanup(func() {
		require.NoError(t, serviceiamsession.Store.DeleteUserSessions(context.Background(), account.UserID))
	})

	require.NoError(t, probeCall(t, conn, "Session", sessionID))

	grpcProbeMu.Lock()
	defer grpcProbeMu.Unlock()
	require.Equal(t, sessionID, grpcProbeSession)
}

// The probe service stands for a project's gRPC service behind the module's
// interceptor: gst.test.Probe with Look for an ordinary authenticated
// action, Open for one declaring Public(), and ChangePassword for the action
// a user must reach while a password change is required. One listener
// serves the whole test binary, the server's registrations being made once.
var (
	grpcProbeOnce    sync.Once
	grpcProbeAddr    string
	errGRPCProbe     error
	grpcProbeMu      sync.Mutex
	grpcProbeCaller  gstgrpc.Caller
	grpcProbeSession string
)

// probeSessionRoute is the route the probe's Session action is registered
// on, the action reading the current session through the framework's call
// function, the way a generated handler runs a service.
const probeSessionRoute = "/api/probe/session"

// probeSessionRsp answers the Session action with the session it read.
type probeSessionRsp struct {
	SessionID string
}

// probeSessionService is the service behind the probe's Session action: it
// reads the current session the way the module's own actions do.
type probeSessionService struct {
	serviceregistry.Base[*modelregistry.Empty, *modelregistry.Empty, *probeSessionRsp]
}

func (*probeSessionService) Get(sc *types.ServiceContext, _ *modelregistry.Empty) (*probeSessionRsp, error) {
	sessionID, _, err := serviceiamsession.CurrentSession(sc)
	if err != nil {
		return nil, err
	}
	return &probeSessionRsp{SessionID: sessionID}, nil
}

// grpcUserAgent is the user agent grpc-go presents for the probe's calls: a
// session a call names has to have been established presenting it, the way
// a request's session has to have been established from its browser.
const grpcUserAgent = "grpc-go/" + grpc.Version

// grpcProbe returns a connection to the probe listener, starting it on
// first use behind the session interceptor TestMain mounted.
func grpcProbe(t *testing.T) *grpc.ClientConn {
	t.Helper()
	grpcProbeOnce.Do(func() {
		handle := func(name string) grpc.MethodDesc {
			return grpc.MethodDesc{
				MethodName: name,
				Handler: func(_ any, ctx context.Context, dec func(any) error, unary grpc.UnaryServerInterceptor) (any, error) {
					in := new(emptypb.Empty)
					if err := dec(in); err != nil {
						return nil, err
					}
					handler := func(ctx context.Context, _ any) (any, error) {
						grpcProbeMu.Lock()
						grpcProbeCaller = gstgrpc.CallerOf(ctx)
						grpcProbeMu.Unlock()
						return &emptypb.Empty{}, nil
					}
					return unary(ctx, in, &grpc.UnaryServerInfo{FullMethod: "/gst.test.Probe/" + name}, handler)
				},
			}
		}
		// Session runs the framework's call function over the probe's
		// Session action, the way a generated handler runs a service, so the
		// call goes through the check of the HTTP-only methods as well.
		serviceregistry.Register[*modelregistry.Empty, *modelregistry.Empty, *probeSessionRsp](consts.Get, probeSessionRoute, &probeSessionService{})
		sessionCall := gstgrpc.ServiceCall[*modelregistry.Empty, *modelregistry.Empty, *probeSessionRsp](consts.Get, probeSessionRoute)
		session := grpc.MethodDesc{
			MethodName: "Session",
			Handler: func(_ any, ctx context.Context, dec func(any) error, unary grpc.UnaryServerInterceptor) (any, error) {
				in := new(emptypb.Empty)
				if err := dec(in); err != nil {
					return nil, err
				}
				handler := func(ctx context.Context, _ any) (any, error) {
					rsp, err := sessionCall(ctx, nil, gstgrpc.Query{}, new(modelregistry.Empty))
					if err != nil {
						return nil, err
					}
					grpcProbeMu.Lock()
					grpcProbeSession = rsp.SessionID
					grpcProbeMu.Unlock()
					return &emptypb.Empty{}, nil
				}
				return unary(ctx, in, &grpc.UnaryServerInfo{FullMethod: "/gst.test.Probe/Session"}, handler)
			},
		}
		grpcserver.Register(func(r grpc.ServiceRegistrar) {
			r.RegisterService(&grpc.ServiceDesc{
				ServiceName: "gst.test.Probe",
				HandlerType: (*any)(nil),
				Methods:     []grpc.MethodDesc{handle("Look"), handle("Open"), handle("ChangePassword"), session},
				Metadata:    "gst/test/probe.proto",
			}, nil)
		},
			grpcserver.Method{Name: "/gst.test.Probe/Look", HTTPMethod: "GET", Route: "/api/probe"},
			grpcserver.Method{Name: "/gst.test.Probe/Open", Public: true},
			grpcserver.Method{Name: "/gst.test.Probe/ChangePassword", HTTPMethod: "POST", Route: "/api/iam/change-password"},
			grpcserver.Method{Name: "/gst.test.Probe/Session", HTTPMethod: "GET", Route: probeSessionRoute},
		)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			errGRPCProbe = err
			return
		}
		addr, _ := listener.Addr().(*net.TCPAddr)
		_ = listener.Close()
		config.App.GRPC = config.GRPC{Listen: "127.0.0.1", Port: addr.Port}
		grpcProbeAddr = addr.String()
		go func() { _ = grpcserver.Run() }()
	})
	require.NoError(t, errGRPCProbe)
	conn, err := grpc.NewClient(grpcProbeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// probeCall invokes the probe rpc name, naming sessionID as the call's
// session when it is not empty.
func probeCall(t *testing.T, conn *grpc.ClientConn, name, sessionID string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if sessionID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+sessionID)
	}
	return conn.Invoke(ctx, "/gst.test.Probe/"+name, &emptypb.Empty{}, &emptypb.Empty{}, grpc.WaitForReady(true))
}

// probeLastCaller returns the caller the probe handler saw on its last call.
func probeLastCaller(t *testing.T) gstgrpc.Caller {
	t.Helper()
	grpcProbeMu.Lock()
	defer grpcProbeMu.Unlock()
	return grpcProbeCaller
}
