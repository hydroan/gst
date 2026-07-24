package versionmod_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil"
	versionmod "github.com/hydroan/gst/module/version"
	"github.com/stretchr/testify/require"
)

var (
	token = "-"
	port  = testutil.SetupRandomServerPort()

	versionAPI = testutil.URL(port, "/api/version")
)

func init() {
	testutil.EnableAutoMigrate()
	os.Setenv(config.DATABASE_TYPE, string(config.DBMySQL))
	os.Setenv(config.MYSQL_USERNAME, "test_module")
	os.Setenv(config.MYSQL_PASSWORD, "test_module")
	os.Setenv(config.MYSQL_DATABASE, "test_module")
	os.Setenv(config.LOGGER_DIR, "./logs")
	os.Setenv(config.AUTH_NONE_EXPIRE_TOKEN, token)
	// Enable audit and sync write before Bootstrap so operationlog test can list logs immediately.
	os.Setenv(config.AUDIT_ENABLED, "true")
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

	testutil.MustWaitForServer(port)
}

func TestVersion(t *testing.T) {
	cli, err := client.New(versionAPI)
	require.NoError(t, err)

	resp, err := cli.Request(http.MethodGet, nil)
	require.NoError(t, err)

	testutil.TestResp(t, resp, func(t *testing.T, rsp *versionmod.VersionRsp) {
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
