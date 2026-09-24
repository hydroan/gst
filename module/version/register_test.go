package versionmod_test

import (
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil"
	versionmod "github.com/hydroan/gst/module/version"
	"github.com/stretchr/testify/require"
)

var baseURL = testutil.BaseURL()

const versionPath = "/api/version"

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Register: versionmod.Register,
	})
}

func TestVersion(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

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
	rsp, err := cli.Get[versionmod.VersionRsp](t.Context(), versionPath)
	require.NoError(t, err)

	require.NotEmpty(t, rsp)
	require.NotEmpty(t, rsp.GoVersion)
	require.NotEmpty(t, rsp.Timestamp)
}

// TestVersionReportsTheBuildTime checks build_time against the build time the
// application records: a frontend compares it to tell a new release from a
// restart, so it must not follow the process start time.
func TestVersionReportsTheBuildTime(t *testing.T) {
	original := config.App.AppInfo.BuildTime
	t.Cleanup(func() { config.App.AppInfo.BuildTime = original })

	built := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	config.App.AppInfo.BuildTime = built
	rsp, err := new(versionmod.VersionService).List(nil, nil)
	require.NoError(t, err)
	require.Equal(t, built.Unix(), rsp.BuildTime)

	// A binary built without a recorded build time reports none.
	config.App.AppInfo.BuildTime = time.Time{}
	rsp, err = new(versionmod.VersionService).List(nil, nil)
	require.NoError(t, err)
	require.Zero(t, rsp.BuildTime)
}
