// Package testsupport is what the demo's test packages share: an account of
// their own, signed up and logged in, to call the routes and rpcs behind
// authentication with.
package testsupport

import (
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// Login signs a fresh account up through the public signup route and logs
// it in; opts go to the client. A test calling rpcs logs in presenting the
// user agent grpc-go sends, client.WithUserAgent(iamtest.GRPCUserAgent),
// since a session is bound to the user agent it was established with.
func Login(t *testing.T, opts ...client.Option) *iamtest.Account {
	t.Helper()
	cli, err := client.New(testutil.BaseURL(), opts...)
	require.NoError(t, err)
	username := "user_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	password := "12345678"
	_, err = cli.Post[iam.SignupRsp](t.Context(), "/api/signup", iam.SignupReq{Username: username, Password: password, RePassword: password})
	require.NoError(t, err)
	return iamtest.Login(t, iam.LoginReq{Username: username, Password: password}, opts...)
}
