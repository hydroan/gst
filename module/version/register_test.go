package versionmod_test

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/helper"
	versionmod "github.com/hydroan/gst/module/version"
	"github.com/stretchr/testify/require"
)

var (
	token = "-"
	port  = 8000

	versionAPI = fmt.Sprintf("http://localhost:%d/api/version", port)
)

func init() {
	os.Setenv(config.DATABASE_TYPE, string(config.DBSqlite))
	os.Setenv(config.SQLITE_IS_MEMORY, "true")
	os.Setenv(config.SERVER_PORT, strconv.Itoa(port))
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)
	// Enable audit and sync write before Bootstrap so operationlog test can list logs immediately.
	os.Setenv(config.AUDIT_ENABLE, "true")
	os.Setenv(config.AUDIT_ASYNC_WRITE, "false")

	if err := bootstrap.Bootstrap(); err != nil {
		panic(err)
	}

	go func() {
		versionmod.Register()

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

func TestVersion(t *testing.T) {
	cli, err := client.New(versionAPI)
	require.NoError(t, err)

	resp, err := cli.Request(http.MethodGet, nil)
	require.NoError(t, err)

	helper.TestResp(t, resp, func(t *testing.T, rsp *versionmod.VersionRsp) {
		t.Helper(
		// #*version.VersionRsp {
		//   +Version     => "" #string
		//   +BuildTime   => 1772694405 #int64
		//   +GitCommit   => "" #string
		//   +GitBranch   => "" #string
		//   +GoVersion   => "go1.25.7" #string
		//   +Environment => "dev" #string
		//   +Uptime      => 1 #int64
		//   +Timestamp   => 1772694406 #int64
		// }
		)

		require.NotEmpty(t, rsp)
		require.NotEmpty(t, rsp.BuildTime)
		require.NotEmpty(t, rsp.GoVersion)
		require.NotEmpty(t, rsp.Timestamp)
	})
}
