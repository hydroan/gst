package iam_test

import (
	"context"
	"fmt"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/goforj/godump"
	"github.com/hydroan/gst/bootstrap"
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
	token = "-"
	port  = testutil.SetupRandomServerPort()

	signupAPI         = testutil.URL(port, "/api/signup")
	loginAPI          = testutil.URL(port, "/api/login")
	logoutAPI         = testutil.URL(port, "/api/logout")
	changepasswordAPI = testutil.URL(port, "/api/iam/change-password")
	resetpasswordAPI  = testutil.URL(port, "/api/iam/reset-password")
	userAPI           = testutil.URL(port, "/api/iam/users")
	currentAPI        = testutil.URL(port, "/api/iam/session/current")
)

func userStatusAPI(userID string) string {
	return testutil.URL(port, fmt.Sprintf("/api/iam/admin/users/%s/status", userID))
}

type ListResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

func init() {
	// NOTE: do not remove me
	godump.Dump()
	testutil.EnableAutoMigrate()
	os.Setenv(config.DATABASE_TYPE, string(config.DBMySQL))
	os.Setenv(config.MYSQL_USERNAME, "test_module")
	os.Setenv(config.MYSQL_PASSWORD, "test_module")
	os.Setenv(config.MYSQL_DATABASE, "test_module")
	os.Setenv(config.REDIS_ENABLED, "true")
	testutil.SetupRandomRedisNamespace()
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)

	iam.Register()
	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}
	seedRootAccount()

	go func() {
		if err := bootstrap.Run(); err != nil {
			panic(err)
		}
	}()

	testutil.MustWaitForServer(port)
}

// seedRootAccount creates the root user and password credential the tests
// authenticate with. Baseline accounts are application data, so the test
// creates them explicitly through the standard database chain and tolerates
// leftovers from previous runs against the shared test database.
func seedRootAccount() {
	ctx := context.Background()

	user := &modeliamuser.User{Username: consts.AUTHZ_USER_ROOT, Status: modeliamuser.UserStatusActive}
	user.ID = consts.AUTHZ_USER_ROOT
	if err := database.Database[*modeliamuser.User](ctx).Create(user); err != nil && !errors.Is(err, database.ErrDuplicatedKey) {
		panic(err)
	}

	credential, err := serviceiamaccount.NewPasswordCredential(ctx, user.ID, rootPassword, false)
	if err != nil {
		panic(err)
	}
	if err := database.Database[*modeliamaccount.PasswordCredential](ctx).Create(credential); err != nil && !errors.Is(err, database.ErrDuplicatedKey) {
		panic(err)
	}
}
