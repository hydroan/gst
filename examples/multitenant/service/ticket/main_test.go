package ticket_test

import (
	"testing"

	// The registrations of main.go: the models, modules, services, jobs,
	// middleware and interceptors register themselves through the init of
	// these packages, and pb the gRPC services.
	_ "multitenant/component"
	_ "multitenant/configx"
	_ "multitenant/cronjob"
	_ "multitenant/interceptor"
	_ "multitenant/leader"
	_ "multitenant/lock"
	_ "multitenant/middleware"
	_ "multitenant/model"
	_ "multitenant/module"
	_ "multitenant/pb"
	"multitenant/router"
	_ "multitenant/service"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/testutil"
)

// TestMain starts the test server of this package the way main.go starts the
// application: the framework bootstraps against sqlite, which needs no
// container, and the redis the iam module keeps its sessions in, which comes
// up in one; the routes are registered, the startup hook seeds the tenants
// and the accounts the tests log in as, and the server serves the tests,
// over HTTP and gRPC, until they are done.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBSqlite,
		Redis:    true,
		Routes:   router.Init,
	})
}
