package iam_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/goforj/godump"
	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/interceptor"
	"github.com/hydroan/gst/internal/grpcserver"
	internalmiddleware "github.com/hydroan/gst/internal/middleware"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/module/iam"
	"github.com/stretchr/testify/require"
)

const rootPassword = "12345678"

var baseURL = testutil.BaseURL()

const (
	signupPath         = "/api/signup"
	loginPath          = "/api/login"
	logoutPath         = "/api/logout"
	changepasswordPath = "/api/iam/change-password"
	resetpasswordPath  = "/api/iam/reset-password"
	currentPath        = "/api/iam/session/current"
)

func adminUserPath(userID string) string {
	return "/api/iam/admin/users/" + userID
}

// sessionClient returns a client that presents the given session id.
func sessionClient(t *testing.T, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(baseURL, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

func TestMain(m *testing.M) {
	// NOTE: do not remove me
	godump.Dump()

	// The root account the tests log in as is seeded the way a deployment's
	// is, by the module from IAM_ROOT_PASSWORD at the start.
	os.Setenv("IAM_ROOT_PASSWORD", rootPassword)
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Redis:    true,
		Register: func() {
			iam.Register()
			// The session check, mounted the way a project mounts it, on
			// both listeners.
			internalmiddleware.RegisterAuth(middleware.IAMSession())
			grpcserver.UseAuth(interceptor.IAMSession())
		},
		Routes: registerRequestMetadataProbe,
	})
}

// TestSeedRootWritesTheRootAccountOnce covers SeedRoot beyond the start of
// this test binary, which seeded the root account the tests log in as from
// IAM_ROOT_PASSWORD: run again with another password it leaves the account
// alone, so the first password still logs in and the second does not; a
// password under the rule a signup's is held to is refused, naming the
// variable; an empty variable writes nothing.
func TestSeedRootWritesTheRootAccountOnce(t *testing.T) {
	t.Setenv("IAM_ROOT_PASSWORD", "another-password")
	require.NoError(t, serviceiamaccount.SeedRoot(t.Context(), nil))
	accountLoginRoot(t)
	accountRequireLoginRejected(t, rbac.UserRoot, "another-password")

	t.Setenv("IAM_ROOT_PASSWORD", "short")
	require.ErrorContains(t, serviceiamaccount.SeedRoot(t.Context(), nil), "IAM_ROOT_PASSWORD")

	t.Setenv("IAM_ROOT_PASSWORD", "")
	require.NoError(t, serviceiamaccount.SeedRoot(t.Context(), nil))
}
