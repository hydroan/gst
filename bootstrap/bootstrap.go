package bootstrap

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/hydroan/gst/authn/jwt"
	"github.com/hydroan/gst/authz/rbac/basic"
	"github.com/hydroan/gst/authz/rbac/tenant"
	"github.com/hydroan/gst/cache"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/controller"
	"github.com/hydroan/gst/cronjob"
	"github.com/hydroan/gst/database/clickhouse"
	"github.com/hydroan/gst/database/helper"
	"github.com/hydroan/gst/database/mysql"
	"github.com/hydroan/gst/database/postgres"
	"github.com/hydroan/gst/database/sqlite"
	"github.com/hydroan/gst/database/sqlserver"
	"github.com/hydroan/gst/debug/gops"
	debugpprof "github.com/hydroan/gst/debug/pprof"
	"github.com/hydroan/gst/debug/statsviz"
	"github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/logger/logrus"
	pkgzap "github.com/hydroan/gst/logger/zap"
	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/provider/cassandra"
	"github.com/hydroan/gst/provider/elastic"
	"github.com/hydroan/gst/provider/etcd"
	"github.com/hydroan/gst/provider/feishu"
	"github.com/hydroan/gst/provider/influxdb"
	"github.com/hydroan/gst/provider/kafka"
	"github.com/hydroan/gst/provider/ldap"
	"github.com/hydroan/gst/provider/memcached"
	"github.com/hydroan/gst/provider/minio"
	"github.com/hydroan/gst/provider/mongo"
	"github.com/hydroan/gst/provider/mqtt"
	"github.com/hydroan/gst/provider/nats"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/provider/rethinkdb"
	"github.com/hydroan/gst/provider/rocketmq"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/task" //nolint:staticcheck
	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap"
)

var (
	initialized bool
	mu          sync.Mutex
)

func Bootstrap() error {
	_, _ = maxprocs.Set(maxprocs.Logger(pkgzap.New("").Infof))

	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	Register(
		config.Init,
		pkgzap.Init,
		logrus.Init,
		prommetrics.Init,

		// cache
		cache.Init,

		// database
		sqlite.Init,
		postgres.Init,
		mysql.Init,
		clickhouse.Init,
		sqlserver.Init,
	)
	if err := Init(); err != nil {
		return err
	}
	// First database drain: create tables and seed records registered before
	// provider/module initialization, typically by model package init functions.
	helper.Wait()

	Register(
		// provider
		redis.Init,
		gstotel.Init,
		elastic.Init,
		mongo.Init,
		minio.Init,
		nats.Init,
		mqtt.Init,
		kafka.Init,
		etcd.Init,
		nats.Init,
		cassandra.Init,
		influxdb.Init,
		memcached.Init,
		rethinkdb.Init,
		rocketmq.Init,
		feishu.Init,
		ldap.Init,

		// Authorization and Authentication
		basic.Init,
		tenant.Init,
		jwt.Init,

		// service
		service.Init,

		controller.Init,
		middleware.Init,
		router.Init,
		grpc.Init,

		// job
		task.Init, //nolint:staticcheck
		cronjob.Init,

		// module system must be the last to be initialized.
		module.Init,
	)

	RegisterCleanup(redis.Close)
	RegisterCleanup(gstotel.Close)
	RegisterCleanup(kafka.Close)
	RegisterCleanup(etcd.Close)
	RegisterCleanup(nats.Close)
	RegisterCleanup(cassandra.Close)
	RegisterCleanup(influxdb.Close)
	RegisterCleanup(memcached.Close)
	RegisterCleanup(rethinkdb.Close)
	RegisterCleanup(rocketmq.Close)
	RegisterCleanup(ldap.Close)
	RegisterCleanup(controller.Clean)
	RegisterCleanup(pkgzap.Clean)
	RegisterCleanup(config.Clean)

	initialized = true

	if err := Init(); err != nil {
		return err
	}

	// module.Init has released module.Use goroutines. Wait for module registration
	// first because modules can call model.Register and enqueue tables/records.
	// This must run before the following helper.Wait; otherwise helper.Wait may
	// check the database queues before modules have added their entries.
	module.Wait()

	// Second database drain: create tables and seed records added by modules
	// during Bootstrap after module.Wait has made those registrations visible.
	helper.Wait()

	return nil
}

func Run() error {
	defer Cleanup()

	// Final pre-server drain for modules registered after Bootstrap but before
	// Run. Keep module.Wait before helper.Wait: late modules may enqueue database
	// tables/records, and helper.Wait can only process entries that already exist.
	// Routes-ready hooks run inside router.Run after this barrier.
	module.Wait()
	helper.Wait()

	RegisterGo(
		router.Run,
		grpc.Run,
		statsviz.Run,
		debugpprof.Run,
		gops.Run,
	)

	RegisterCleanup(router.Stop)
	RegisterCleanup(grpc.Stop)
	RegisterCleanup(statsviz.Stop)
	RegisterCleanup(debugpprof.Stop)
	RegisterCleanup(gops.Stop)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	errCh := make(chan error, 1)

	go func() {
		errCh <- Go()
	}()
	select {
	case sig := <-sigCh:
		zap.S().Infow("canceled by signal", "signal", sig)
		return nil
	case err := <-errCh:
		return err
	}
}
