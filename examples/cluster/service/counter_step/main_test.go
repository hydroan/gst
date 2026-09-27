package counterstep_test

import (
	"testing"

	// The application registers its models, services, jobs, leader work,
	// locks, interceptors and gRPC services through the init of these
	// packages, as main.go imports them; the replicated cache component is
	// left out, since it needs kafka and these tests do not.
	_ "cluster/configx"
	_ "cluster/cronjob"
	_ "cluster/interceptor"
	_ "cluster/leader"
	_ "cluster/lock"
	_ "cluster/middleware"
	_ "cluster/model"
	_ "cluster/module"
	_ "cluster/pb"
	"cluster/router"
	_ "cluster/service"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/testutil"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBSqlite,
		Redis:    true,
		Routes:   router.Init,
	})
}
