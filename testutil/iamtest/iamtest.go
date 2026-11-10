// Package iamtest logs a test in through the iam module, the way a client of
// the project does, and hands it the session: as the cookie the returned
// client carries on every request, and as the session id a gRPC call names
// with grpc.WithBearer. A session is bound to the user agent that logged in,
// so a session meant for gRPC calls is established presenting the agent
// grpc-go sends, GRPCUserAgent, which is what GRPCSession does.
//
//	root := iam.LoginReq{Username: rbac.UserRoot, Password: password}
//	account := iamtest.Login(t, root)
//	rsp, err := account.Client.Get[model.RecordListRsp](t.Context(), "/api/records")
//
//	conn := testutil.DialGRPC(t)
//	ctx := grpc.WithBearer(t.Context(), iamtest.GRPCSession(t, root))
//	reply, err := pb.NewRecordServiceClient(conn).ListRecord(ctx, &pb.ListRecordRequest{})
package iamtest

import (
	"net/http"
	"testing"

	"github.com/hydroan/gst/client"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/iam"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// GRPCUserAgent is the user agent grpc-go presents on every call. A session
// is bound to the user agent it was established with, so the account a gRPC
// call names logs in presenting this one, see GRPCSession.
const GRPCUserAgent = "grpc-go/" + grpc.Version

// loginPath is the route the iam module serves a login at.
const loginPath = "/api/login"

// Account is an account logged in. Client carries the session cookie, so
// every request through it is authenticated and acts in TenantID, the tenant
// the login chose; SessionID is the session itself, for a gRPC call or a
// request made by hand; UserID is the id of the user's row, as the login
// answered it, and Username the name it logged in with.
type Account struct {
	Client    *client.Client
	UserID    string
	Username  string
	TenantID  string
	SessionID string
}

// Login logs req in and returns the account; opts configure the client,
// client.WithUserAgent among them. A refused login fails the test.
func Login(t *testing.T, req iam.LoginReq, opts ...client.Option) *Account {
	t.Helper()

	cli, err := client.New(testutil.BaseURL(), opts...)
	require.NoError(t, err)
	envelope, err := cli.Do(t.Context(), http.MethodPost, loginPath, req)
	require.NoError(t, err)
	login := testutil.DecodeResp[iam.LoginRsp](t, envelope)
	cookie := envelope.Cookie(serviceiamsession.SessionCookieName)
	require.NotNil(t, cookie, "the login set no session cookie")
	return &Account{Client: cli, UserID: login.Principal.UserID, Username: req.Username, TenantID: req.TenantID, SessionID: cookie.Value}
}

// GRPCSession logs req in presenting GRPCUserAgent and returns the session
// id, the credential a gRPC call names with grpc.WithBearer.
func GRPCSession(t *testing.T, req iam.LoginReq) string {
	t.Helper()

	return Login(t, req, client.WithUserAgent(GRPCUserAgent)).SessionID
}
