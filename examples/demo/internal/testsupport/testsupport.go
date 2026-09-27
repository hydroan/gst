// Package testsupport is what the demo's test packages share: an account of
// their own, signed up and logged in, to call the routes and rpcs behind
// authentication with, and the connection the gRPC tests dial.
package testsupport

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GRPCUserAgent is the user agent grpc-go presents. A session is bound to
// the user agent it was established with, so the account a gRPC call names
// logs in presenting this one.
const GRPCUserAgent = "grpc-go/" + grpc.Version

// Account is an account signed up for a test and logged in: Client holds
// its session cookie, so every request through it is authenticated, and
// SessionID is the session itself, for a gRPC call or a request made by
// hand.
type Account struct {
	Client    *client.Client
	UserID    string
	Username  string
	SessionID string
}

// Login signs a fresh account up through the public signup route and logs
// it in; opts go to the client.
func Login(t *testing.T, opts ...client.Option) *Account {
	t.Helper()
	cli, err := client.New(testutil.BaseURL(), opts...)
	require.NoError(t, err)
	username := "user_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	password := "12345678"
	_, err = cli.Post[iam.SignupRsp](t.Context(), "/api/signup", iam.SignupReq{Username: username, Password: password, RePassword: password})
	require.NoError(t, err)
	envelope, err := cli.Do(t.Context(), http.MethodPost, "/api/login", iam.LoginReq{Username: username, Password: password})
	require.NoError(t, err)
	login := testutil.DecodeResp[iam.LoginRsp](t, envelope)
	cookie := envelope.Cookie("session_id")
	require.NotNil(t, cookie, "the login set no session cookie")
	return &Account{Client: cli, UserID: login.Principal.UserID, Username: username, SessionID: cookie.Value}
}

// Session logs a fresh account in presenting GRPCUserAgent and returns the
// session id its gRPC calls name, see Authorized.
func Session(t *testing.T) string {
	t.Helper()
	return Login(t, client.WithUserAgent(GRPCUserAgent)).SessionID
}

// Authorized returns ctx naming the session for a gRPC call, as the
// metadata "authorization: Bearer <session id>".
func Authorized(ctx context.Context, sessionID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+sessionID)
}

// Dial connects to the test server's gRPC listener, plaintext, closing the
// connection when the test ends.
func Dial(t *testing.T) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(testutil.GRPCTarget(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
