// Package testsupport logs the tests in and connects them to the test server.
package testsupport

import (
	"context"
	"net/http"
	"testing"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GRPCUserAgent is the user agent grpc-go sends; a session is bound to the
// user agent it was created with, so a login meant for gRPC calls presents
// it.
const GRPCUserAgent = "grpc-go/" + grpc.Version

// RootPassword is the password TestMain seeds the root account with.
const RootPassword = "root-for-tests"

// Account is a logged-in root: the client carrying its session cookie and
// the session id its gRPC calls name.
type Account struct {
	Client    *client.Client
	Username  string
	SessionID string
}

// Login logs the root account in and returns it.
func Login(t *testing.T, opts ...client.Option) *Account {
	t.Helper()
	cli, err := client.New(testutil.BaseURL(), opts...)
	require.NoError(t, err)
	envelope, err := cli.Do(t.Context(), http.MethodPost, "/api/login", iam.LoginReq{Username: rbac.UserRoot, Password: RootPassword})
	require.NoError(t, err)
	cookie := envelope.Cookie("session_id")
	require.NotNil(t, cookie, "the login set no session cookie")
	return &Account{Client: cli, Username: rbac.UserRoot, SessionID: cookie.Value}
}

// Session logs the root account in presenting GRPCUserAgent and returns the
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
