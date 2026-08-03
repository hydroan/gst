package main_test

import (
	"testing"

	// The application registers its models, modules, services and cron jobs
	// through the init of these packages, exactly as main.go imports them.
	_ "demo/configx"
	_ "demo/cronjob"
	_ "demo/middleware"
	"demo/model"
	_ "demo/model"
	_ "demo/module"
	"demo/router"
	_ "demo/service"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

var pingAPI = testutil.URL("/api/pings")

// TestMain mirrors main.go: the framework bootstraps first, the routes are
// registered second, and the server serves last. Seed is where router.Init
// belongs, because registering a route needs a bootstrapped framework.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Redis:    true,
		Seed:     func() { util.RunOrDie(router.Init) },
	})
}

// TestPing exercises the whole stack a gst application is made of: the request
// reaches a generated route, the route reaches the service, and the service
// reads from the database the test suite prepared.
func TestPing(t *testing.T) {
	cli, err := client.New(pingAPI)
	require.NoError(t, err)

	resp, err := cli.Request("GET", nil)
	require.NoError(t, err)

	testutil.TestResp(t, resp, func(t *testing.T, rsp *model.PingRsp) {
		t.Helper()
		require.NotNil(t, rsp)
	})
}
