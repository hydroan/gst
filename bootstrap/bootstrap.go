// Package bootstrap brings a gst process up and takes it down again.
//
// Bootstrap runs the setup every process needs before it can serve —
// configuration, logging and metrics, the databases, the backbone clients,
// the providers, the authorization, service, controller, middleware and
// router layers, and the modules — and returns once the tables exist and
// the providers are up. Run starts the listeners and the components that
// run alongside them, blocks until a termination signal or a listener
// failure, and tears everything down in reverse. The generated entry point
// drives both, with the project's route registration in between; a test
// harness drives them the same way.
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

// Bootstrap runs once per process: a later call returns at once.
var (
	mu          sync.Mutex
	initialized bool
)

// processCtx is the context every lifecycle component starts on. It is
// canceled the moment shutdown begins, so background work derived from it
// winds down on its own.
var processCtx, cancelProcess = context.WithCancel(context.Background())

// componentStopTimeout bounds how long the lifecycle may take to stop what
// it started at shutdown — the components' in-flight work first, then the
// providers, all within the one window — so a stuck job cannot hold the
// shutdown hostage. It matches the bound router.Stop gives the HTTP drain.
const componentStopTimeout = 30 * time.Second

// Bootstrap brings up everything the process needs before it can serve, in
// dependency order: configuration, logging and metrics; the databases; the
// backbone clients and the providers; the authorization, service,
// controller, middleware and router layers; the modules last. It returns
// once every table registered so far exists and every enabled provider is
// up, so whatever runs between Bootstrap and Run — a test harness seeding
// data — and the routes-ready hooks, which Run fires first, can rely on
// them. A failure is fatal to the process: the entry point exits on it.
func Bootstrap() error {
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return nil
	}

	startup.Register(
		config.Init,
		pkgzap.Init,
		prommetrics.Init,

		// database
		sqlite.Init,
		postgres.Init,
		mysql.Init,
		clickhouse.Init,
	)
	if err := startup.Init(); err != nil {
		return err
	}
	// Registered first so they run last: every cleanup after them logs what
	// it did, and a line written once the log writers have stopped never
	// reaches its file. The temp directory goes right before the writers,
	// for the same reason.
	registerCleanup(pkgzap.Clean)
	registerCleanup(config.Clean)
	warnUnlinkedProviders()
	// First database drain: create the tables registered before the clients
	// and modules initialize, typically by model package init functions.
	dbruntime.Wait()

	startup.Register(
		// backbone clients
		redis.Init,
		gstotel.Init,
	)
	if err := startup.Init(); err != nil {
		return err
	}

	// The providers — the clients of external systems that joined the
	// lifecycle registry from their package init functions — come up right
	// after the backbone clients, so the layers below and everything that
	// runs between Bootstrap and Run can use them.
	if err := lifecycle.Start(processCtx, lifecycle.StageProvider); err != nil {
		return err
	}

	startup.Register(
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

	if err := startup.Init(); err != nil {
		return err
	}

	// module.Init has released module.Use goroutines. Wait for module
	// registration first because modules can register models and enqueue
	// tables. This must run before the following database drain; otherwise
	// dbruntime.Wait may check the database queues before modules have added
	// their entries.
	module.Wait()

	// Second database drain: create the tables added by modules during
	// Bootstrap, after module.Wait has made those registrations visible.
	dbruntime.Wait()

	// Mark success only after every phase finished: a failed Bootstrap must
	// keep returning its error instead of turning into a silent nil on a
	// retry. Bootstrap is single-shot; callers exit on failure (RunOrDie).
	initialized = true

	return nil
}

// Run starts the listeners and the components that run alongside them, then
// blocks until the process is told to stop. A termination signal stops it
// cleanly: readiness goes down first, the components stop taking on work,
// the configured drain window passes, and everything is torn down in the
// reverse order of its setup. A listener that fails ends it the same way,
// with the failure as the error, so the process never runs on with nothing
// to report.
func Run() error {
	defer clean()
	// The providers Bootstrap started are stopped on every way out of Run,
	// the ways that fail before the components start included. Registered
	// first, so that LIFO runs it right after the HTTP drain: in-flight
	// jobs finish while the connections they may be using are still open,
	// and the providers close after their last user.
	registerCleanup(stopLifecycle)

	// Final pre-server drain for modules registered after Bootstrap but
	// before Run. Keep module.Wait before dbruntime.Wait: late modules may
	// enqueue tables, and dbruntime.Wait can only process entries that
	// already exist.
	module.Wait()
	dbruntime.Wait()

	// The routes-ready hooks run right after the last barrier and before
	// anything that could use what they seed: the components below — a
	// scheduler catching up an instant on start-up would otherwise race the
	// seeding — and the listener after them. Across the deployment they run
	// one process at a time, under the startup lock on the primary database:
	// seeding reads before it writes, and replicas starting together would
	// each find nothing and each write. A termination signal is watched
	// from here on, through the context the hooks run on: one that arrives
	// while the process waits its turn ends the wait, and one that arrives
	// during the hooks ends the statements in flight and the hooks with
	// them, both the way a signal after the start does — cleanly, with
	// nothing to report. A hook that fails ends Run the way a failing
	// listener would.
	starting, stopWatching := signal.NotifyContext(processCtx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	err := dbruntime.Serialized(starting, "seed", func() error { return router.RunRoutesReadyHooks(starting) })
	// The channel awaitShutdown reads takes over the signals before the
	// start's watch stops: with no channel registered the signals fall back
	// to their default disposition, and one arriving then would kill the
	// process without any of the teardown below. Read the start's outcome
	// before its watch stops too: stopping it ends the context as well.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	signaled := starting.Err() != nil && processCtx.Err() == nil
	stopWatching()
	if signaled {
		zap.S().Infow("stopped while starting")
		return nil
	}
	if err != nil {
		return err
	}

	// The components that run alongside the server — the scheduler and its
	// kind — start here, after the hooks: every table they may touch exists
	// and is seeded, and the listener opens right after.
	if err = lifecycle.Start(processCtx, lifecycle.StageComponent); err != nil {
		return err
	}

	startup.RegisterGo(
		router.Run,
		statsviz.Run,
		debugpprof.Run,
		gops.Run,
	)

	registerCleanup(router.Stop)
	registerCleanup(statsviz.Stop)
	registerCleanup(debugpprof.Stop)
	registerCleanup(gops.Stop)

	err = awaitShutdown(startup.Go(), lifecycle.Failure(), sigCh)

	// Either way the process leaves the same way: stop answering readiness
	// before anything is torn down, cancel the process context so the
	// components stop taking on new work, then hold there for the
	// configured window. Teardown starts when it elapses.
	controller.Probe.Drain()
	cancelProcess()
	awaitDrain(sigCh)
	return err
}

// awaitShutdown blocks until the process is told to stop — one of the
// long-running functions failing (listeners is the context startup.Go
// cancels with the failure as its cause), a component failing (components,
// see lifecycle.Fail) or a termination signal — and returns the failure, nil
// for a signal. Returning is what stops what is still serving: Run's
// deferred clean shuts it down.
func awaitShutdown(listeners, components context.Context, sigCh <-chan os.Signal) error {
	select {
	case sig := <-sigCh:
		zap.S().Infow("canceled by signal", "signal", sig)
		return nil
	case <-listeners.Done():
		err := context.Cause(listeners)
		zap.S().Errorw("shutting down after a failure", "err", err)
		return err
	case <-components.Done():
		err := context.Cause(components)
		zap.S().Errorw("shutting down after a component failure", "err", err)
		return err
	}
}

// stopLifecycle cancels the process context, so any component still taking
// on work stops doing so, and stops the started components with a bounded
// wait for their in-flight work.
func stopLifecycle() {
	cancelProcess()
	ctx, cancel := context.WithTimeout(context.Background(), componentStopTimeout)
	defer cancel()
	lifecycle.Stop(ctx)
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
