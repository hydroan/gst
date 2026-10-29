// Package testsupport is what the test packages share: the root account the
// startup seeds, logged in, to call the routes and rpcs behind the session
// check and the authorization with, and the connection the gRPC tests dial.
package testsupport

import (
	"context"
	"net/http"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/consts"
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

// RootPassword is the password every test package's TestMain names for the
// startup to seed the root account with, the one Login presents.
const RootPassword = "root-for-tests"

// Account is the root account logged in: Client holds its session cookie,
// so every request through it is authenticated, and SessionID is the
// session itself, for a gRPC call or a request made by hand.
type Account struct {
	Client    *client.Client
	Username  string
	SessionID string
}

// Login logs the root account in and returns it; opts go to the client. The
// authorization mounted behind the session check refuses a subject holding
// no role, and root holds the system role that reaches every route, which
// is what lets the tests call the routes without appointing anyone.
func Login(t *testing.T, opts ...client.Option) *Account {
	t.Helper()
	cli, err := client.New(testutil.BaseURL(), opts...)
	require.NoError(t, err)
	envelope, err := cli.Do(t.Context(), http.MethodPost, "/api/login", iam.LoginReq{Username: consts.AUTHZ_USER_ROOT, Password: RootPassword})
	require.NoError(t, err)
	cookie := envelope.Cookie("session_id")
	require.NotNil(t, cookie, "the login set no session cookie")
	return &Account{Client: cli, Username: consts.AUTHZ_USER_ROOT, SessionID: cookie.Value}
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
