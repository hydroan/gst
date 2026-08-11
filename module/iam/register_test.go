package iam_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/goforj/godump"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/types/consts"
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

func userStatusPath(userID string) string {
	return fmt.Sprintf("/api/iam/admin/users/%s/status", userID)
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

	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Redis:    true,
		Register: func() { iam.Register() },
		Routes:   registerRequestMetadataProbe,
		Seed:     seedRootAccount,
	})
}

// seedRootAccount creates the root user and password credential the tests
// authenticate with. Baseline accounts are application data, so the test
// creates them explicitly through the standard database chain.
func seedRootAccount() {
	ctx := context.Background()

	user := &modeliamuser.User{Username: consts.AUTHZ_USER_ROOT, Status: modeliamuser.UserStatusActive}
	user.ID = consts.AUTHZ_USER_ROOT
	if err := database.Database[*modeliamuser.User](ctx).Create(user); err != nil {
		panic(err)
	}

	credential, err := serviceiamaccount.NewPasswordCredential(ctx, user.ID, rootPassword, false)
	if err != nil {
		panic(err)
	}
	if err := database.Database[*modeliamaccount.PasswordCredential](ctx).Create(credential); err != nil {
		panic(err)
	}
}
