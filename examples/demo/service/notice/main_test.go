package notice_test

import (
	"os"
	"testing"

	"demo/configx"

	// The registrations of main.go: the models, modules, services, jobs,
	// middleware and interceptors register themselves through the init of
	// these packages, and pb the gRPC services.
	_ "demo/component"
	_ "demo/configx"
	_ "demo/cronjob"
	_ "demo/interceptor"
	_ "demo/leader"
	_ "demo/lock"
	_ "demo/middleware"
	_ "demo/model"
	_ "demo/module"
	_ "demo/pb"
	"demo/router"
	_ "demo/service"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/testutil"
)

// TestMain starts the test server of this package the way main.go starts the
// application: the framework bootstraps against sqlite, which needs no
// container, and the redis the iam module keeps its sessions in, which comes
// up in one; the routes are registered, and the server serves the tests,
// over HTTP and gRPC, until they are done.
func TestMain(m *testing.M) {
	// The stream sends NOTICE_COUNT events; two keep the assertion short.
	os.Setenv(configx.NOTICE_COUNT, "2")
	testutil.Run(m, testutil.Server{
		Database: config.DBSqlite,
		Redis:    true,
		Routes:   router.Init,
	})
}
