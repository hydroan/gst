package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hydroan/gst/authz/rbac"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database/clickhouse"
	"github.com/hydroan/gst/database/mysql"
	"github.com/hydroan/gst/database/postgres"
	"github.com/hydroan/gst/database/sqlite"
	"github.com/hydroan/gst/debug/gops"
	debugpprof "github.com/hydroan/gst/debug/pprof"
	"github.com/hydroan/gst/debug/statsviz"
	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/lifecycle"
	pkgzap "github.com/hydroan/gst/logger/zap"
	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/module"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/service"
	"go.uber.org/zap"
)

var (
	initialized bool
	mu          sync.Mutex
)

// componentStopTimeout bounds how long a lifecycle stage may take to finish
// its in-flight work at shutdown, so a stuck job cannot hold the shutdown
// hostage. It matches the bound router.Stop gives the HTTP drain.
const componentStopTimeout = 30 * time.Second

// processCtx is the context every lifecycle component starts on. It is
// canceled the moment shutdown begins, so background work derived from it
// winds down on its own.
var processCtx, cancelProcess = context.WithCancel(context.Background())

// stopLifecycle cancels the process context, so any component still taking
// on work stops doing so, and stops the started components with a bounded
// wait for their in-flight work.
func stopLifecycle() {
	cancelProcess()
	ctx, cancel := context.WithTimeout(context.Background(), componentStopTimeout)
	defer cancel()
	lifecycle.Stop(ctx)
}

func Bootstrap() error {
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	ins.Register(
		config.Init,
		pkgzap.Init,
		prommetrics.Init,

		// database
		sqlite.Init,
		postgres.Init,
		mysql.Init,
		clickhouse.Init,
	)
	if err := ins.Init(); err != nil {
		return err
	}
	// First database drain: create tables and seed records registered before
	// provider/module initialization, typically by model package init functions.
	dbruntime.Wait()

	ins.Register(
		// backbone providers
		redis.Init,
		gstotel.Init,
	)

	ins.Register(
		// Authorization and Authentication
		rbac.Init,

		// service
		service.Init,

		controller.Init,
		middleware.Init,
		router.Init,

		// module system must be the last to be initialized.
		module.Init,
	)

	registerCleanup(closeComponent("redis", redis.Close))
	registerCleanup(closeComponent("otel", gstotel.Close))
	registerCleanup(controller.Clean)
	registerCleanup(pkgzap.Clean)
	registerCleanup(config.Clean)

	if err := ins.Init(); err != nil {
		return err
	}

	// module.Init has released module.Use goroutines. Wait for module registration
	// first because modules can call model.Register and enqueue tables/records.
	// This must run before the following database drain; otherwise dbruntime.Wait may
	// check the database queues before modules have added their entries.
	module.Wait()

	// Second database drain: create tables and seed records added by modules
	// during Bootstrap after module.Wait has made those registrations visible.
	dbruntime.Wait()

	// Mark success only after every phase finished: a failed Bootstrap must
	// keep returning its error instead of turning into a silent nil on a
	// retry. Bootstrap is single-shot; callers exit on failure (RunOrDie).
	initialized = true

	return nil
}

func Run() error {
	defer clean()

	// Final pre-server drain for modules registered after Bootstrap but before
	// Run. Keep module.Wait before dbruntime.Wait: late modules may enqueue database
	// tables/records, and dbruntime.Wait can only process entries that already exist.
	// Routes-ready hooks run inside router.Run after this barrier.
	module.Wait()
	dbruntime.Wait()

	// The lifecycle components — the providers, then the scheduler and its
	// kind — start here, after the last barrier: every table they may touch
	// exists, and the listener opens right after. Their cleanup is
	// registered before the listener's, so LIFO stops them right after the
	// HTTP drain: in-flight jobs finish while the connections they may be
	// using are still open, and the providers close after their last user.
	if err := lifecycle.Start(processCtx); err != nil {
		stopLifecycle()
		return err
	}
	registerCleanup(stopLifecycle)

	ins.RegisterGo(
		router.Run,
		statsviz.Run,
		debugpprof.Run,
		gops.Run,
	)

	registerCleanup(router.Stop)
	registerCleanup(statsviz.Stop)
	registerCleanup(debugpprof.Stop)
	registerCleanup(gops.Stop)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	failed := ins.Go()
	select {
	case sig := <-sigCh:
		zap.S().Infow("canceled by signal", "signal", sig)
		// Stop answering readiness before anything is torn down, cancel the
		// process context so the components stop taking on new work, then
		// hold there for the configured window. Teardown starts when it
		// elapses.
		controller.Probe.Drain()
		cancelProcess()
		awaitDrain(sigCh)
		return nil
	case <-failed.Done():
		// One of the long-running functions failed. Returning is what stops
		// the others: the deferred clean shuts down those still serving.
		return context.Cause(failed)
	}
}

// awaitDrain holds the process in its not-ready state for the configured
// delay. That window is what a load balancer routing by readiness needs to
// notice this process dropped out and stop opening connections to it; without
// it, the listener can start refusing connections the balancer is still
// sending. A second signal ends the wait, so an operator can always cut a
// drain short.
func awaitDrain(sigCh <-chan os.Signal) {
	delay := config.App.Server.ShutdownDelay
	if delay <= 0 {
		return
	}

	zap.S().Infow("draining before shutdown", "delay", delay)
	select {
	case <-time.After(delay):
	case sig := <-sigCh:
		zap.S().Infow("drain cut short by signal", "signal", sig)
	}
}
