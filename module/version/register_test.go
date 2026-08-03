package versionmod_test

import (
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil"
	versionmod "github.com/hydroan/gst/module/version"
	"github.com/stretchr/testify/require"
)

var baseURL = testutil.URL("")

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
	rsp, err := client.Get[versionmod.VersionRsp](cli, versionPath)
	require.NoError(t, err)

	require.NotEmpty(t, rsp)
	require.NotEmpty(t, rsp.BuildTime)
	require.NotEmpty(t, rsp.GoVersion)
	require.NotEmpty(t, rsp.Timestamp)
}
