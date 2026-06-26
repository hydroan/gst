package iam_test

import (
	"os"

	"github.com/goforj/godump"
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/iam"
)

var (
	token = "-"
	port  = testutil.SetupRandomServerPort()

	signupAPI         = testutil.URL(port, "/api/signup")
	loginAPI          = testutil.URL(port, "/api/login")
	logoutAPI         = testutil.URL(port, "/api/logout")
	changepasswordAPI = testutil.URL(port, "/api/iam/change-password")
	resetpasswordAPI  = testutil.URL(port, "/api/iam/reset-password")
	accountstatusAPI  = testutil.URL(port, "/api/iam/account-status")
	userAPI           = testutil.URL(port, "/api/iam/users")
	currentAPI        = testutil.URL(port, "/api/iam/session/current")
)

type ListResponse[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

func init() {
	// NOTE: do not remove me
	godump.Dump()
	os.Setenv(config.DATABASE_TYPE, string(config.DBSqlite))
	os.Setenv(config.SQLITE_IS_MEMORY, "true")
	os.Setenv(config.REDIS_ENABLE, "true")
	testutil.SetupRandomRedisNamespace()
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)

	iam.Register()
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
