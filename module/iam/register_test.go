package iam_test

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/goforj/godump"
	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/module/iam"
)

var (
	token = "-"
	port  = 8000

	signupAPI         = fmt.Sprintf("http://localhost:%d/api/signup", port)
	loginAPI          = fmt.Sprintf("http://localhost:%d/api/login", port)
	logoutAPI         = fmt.Sprintf("http://localhost:%d/api/logout", port)
	changepasswordAPI = fmt.Sprintf("http://localhost:%d/api/iam/change-password", port)
	resetpasswordAPI  = fmt.Sprintf("http://localhost:%d/api/iam/reset-password", port)
	accountstatusAPI  = fmt.Sprintf("http://localhost:%d/api/iam/account-status", port)
	userAPI           = fmt.Sprintf("http://localhost:%d/api/iam/users", port)
	groupAPI          = fmt.Sprintf("http://localhost:%d/api/iam/groups", port)
	tenantAPI         = fmt.Sprintf("http://localhost:%d/api/iam/tenants", port)
	currentAPI        = fmt.Sprintf("http://localhost:%d/api/iam/session/current", port)
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
	os.Setenv(config.SERVER_PORT, strconv.Itoa(port))
	os.Setenv(config.REDIS_ENABLE, "true")
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)

	iam.Register(iam.Config{EnableTenant: true})
	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}

	go func() {
		if err := bootstrap.Run(); err != nil {
			panic(err)
		}
	}()

	for {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			l.Close()
			time.Sleep(1 * time.Second)
			continue
		}
		if errors.Is(err, syscall.EADDRINUSE) {
			break
		}
		panic(err)

	}
}
