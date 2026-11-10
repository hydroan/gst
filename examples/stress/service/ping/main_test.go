package ping_test

import (
	"os"
	"testing"

	// The registrations of main.go: the models, modules, services and
	// interceptors register themselves through the init of these packages.
	_ "stress/component"
	"stress/configx"
	_ "stress/cronjob"
	_ "stress/interceptor"
	"stress/internal/testsupport"
	_ "stress/leader"
	_ "stress/lock"
	_ "stress/middleware"
	_ "stress/model"
	_ "stress/module"
	_ "stress/pb"
	"stress/router"
	_ "stress/service"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/testutil"
)

func TestMain(m *testing.M) {
	// The startup seeds the accounts the tests log in as, with the password
	// the environment names; the sessions live in Redis.
	os.Setenv("IAM_ROOT_PASSWORD", testsupport.RootPassword)
	os.Setenv(configx.SEED_PASSWORD, testsupport.RootPassword)
	testutil.Run(m, testutil.Server{
		Database: config.DBSqlite,
		Redis:    true,
		Routes:   router.Init,
	})
}
