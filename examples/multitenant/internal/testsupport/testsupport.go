// Package testsupport is what the test packages share: the seeded accounts
// logged in, in a tenant or in none, to call the routes and rpcs behind the
// session check and the authorization with, and the connection the gRPC
// tests dial.
package testsupport

import (
	"context"
	"net/http"
	"testing"

	"multitenant/configx"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
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

// Account is a seeded account logged in: Client holds its session cookie,
// so every request through it is authenticated and acts in TenantID, and
// SessionID is the session itself, for a gRPC call or a request made by
// hand.
type Account struct {
	Client    *client.Client
	UserID    string
	Username  string
	TenantID  string
	SessionID string
}

// Login logs the seeded account username in, choosing the tenant to act in
// ("" for none, which the authorization treats as the default tenant), and
// returns it; opts go to the client. The password is the one the seed
// section configures, which every seeded account starts with.
func Login(t *testing.T, username, tenantID string, opts ...client.Option) *Account {
	t.Helper()
	cli, err := client.New(testutil.BaseURL(), opts...)
	require.NoError(t, err)
	envelope, err := cli.Do(t.Context(), http.MethodPost, "/api/login", iam.LoginReq{
		Username: username,
		Password: config.Get[configx.Seed]().Password,
		TenantID: tenantID,
	})
	require.NoError(t, err)
	login := testutil.DecodeResp[iam.LoginRsp](t, envelope)
	cookie := envelope.Cookie("session_id")
	require.NotNil(t, cookie, "the login set no session cookie")
	return &Account{Client: cli, UserID: login.Principal.UserID, Username: username, TenantID: tenantID, SessionID: cookie.Value}
}

// Session logs the seeded account in presenting GRPCUserAgent and returns
// the session id its gRPC calls name, see Authorized.
func Session(t *testing.T, username, tenantID string) string {
	t.Helper()
	return Login(t, username, tenantID, client.WithUserAgent(GRPCUserAgent)).SessionID
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
