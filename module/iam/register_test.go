package iam_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/goforj/godump"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/types/consts"
)

const rootPassword = "12345678"

var (
	signupAPI         = testutil.URL("/api/signup")
	loginAPI          = testutil.URL("/api/login")
	logoutAPI         = testutil.URL("/api/logout")
	changepasswordAPI = testutil.URL("/api/iam/change-password")
	resetpasswordAPI  = testutil.URL("/api/iam/reset-password")
	userAPI           = testutil.URL("/api/iam/users")
	currentAPI        = testutil.URL("/api/iam/session/current")
)

func userStatusAPI(userID string) string {
	return testutil.URL(fmt.Sprintf("/api/iam/admin/users/%s/status", userID))
}

func TestMain(m *testing.M) {
	// NOTE: do not remove me
	godump.Dump()

	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Redis:    true,
		Register: func() { iam.Register() },
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
