package iam_test

import (
	"fmt"
	"os"

	"github.com/goforj/godump"
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
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
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

func init() {
	// NOTE: do not remove me
	godump.Dump()
	os.Setenv(config.DATABASE_TYPE, string(config.DBMySQL))
	os.Setenv(config.MYSQL_USERNAME, "test_module")
	os.Setenv(config.MYSQL_PASSWORD, "test_module")
	os.Setenv(config.MYSQL_DATABASE, "test_module")
	os.Setenv(config.REDIS_ENABLED, "true")
	testutil.SetupRandomRedisNamespace()
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)

	iam.Register(iam.Config{
		DefaultUsers: []*iam.DefaultUser{
			{
				ID:       consts.AUTHZ_USER_ROOT,
				Username: consts.AUTHZ_USER_ROOT,
				Password: rootPassword,
			},
		},
	})
	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}

	go func() {
		if err := bootstrap.Run(); err != nil {
			panic(err)
		}
	}()

	testutil.MustWaitForServer(port)
}
