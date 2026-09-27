package bootstrap

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/hydroan/gst/internal/lifecycle"
	"go.uber.org/zap"
)

// The cleanup stack: whatever Bootstrap and Run bring up registers how it is
// torn down, and clean unwinds the stack once at shutdown.
var (
	cleanups  []func()
	cleanOnce sync.Once
)

// failNowTimeout bounds the teardown of a process that fails now, see
// lifecycle.FailNow. Nothing left in it then waits for work — stopping the
// providers, flushing the logs — so it takes a moment; the bound is for a
// cleanup that hangs, which is left behind so the process can exit. A
// variable so a test can play the bound out in milliseconds.
var failNowTimeout = 10 * time.Second

// clean runs the cleanup stack once; later calls do nothing. Once the process
// fails now, it waits for the stack no longer than failNowTimeout.
func clean() {
	cleanOnce.Do(func() {
		unwound := make(chan struct{})
		go func() {
			defer close(unwound)
			runCleanups()
		}()

		failedNow := lifecycle.FailedNow()
		select {
		case <-unwound:
			return
		case <-failedNow.Done():
		}
		select {
		case <-unwound:
		case <-time.After(failNowTimeout):
			zap.S().Errorw("gave up on the teardown after a failure the shutdown must not wait on",
				"timeout", failNowTimeout, "err", context.Cause(failedNow))
		}
	})
}

// registerCleanup pushes a cleanup onto the stack. Cleanups run serially in
// reverse registration order (LIFO) when the process shuts down: register
// dependencies first and dependents later, exactly like defer.
func registerCleanup(cleanup func()) {
	cleanups = append(cleanups, cleanup)
}

// runCleanups runs the stack serially in reverse registration order:
// teardown mirrors setup the way a defer stack unwinds, so whatever was
// brought up last (the HTTP listener draining in-flight requests) is torn
// down first, and what everything else depends on (connections, log
// writers, the temp directory) is torn down after the drain finished.
func runCleanups() {
	for _, cleanup := range slices.Backward(cleanups) {
		runSafe(cleanup)
	}
}

// runSafe runs one cleanup, reporting a panic on stderr instead of letting
// it end the shutdown halfway.
func runSafe(cleanup func()) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup handler error:", err)
		}
	}()

	cleanup()
}

// stopTogether runs the stops side by side, each on abandon bounded by the
// one window of lifecycle.StopTimeout all of them share, and returns once
// every one returned: the HTTP listener and the gRPC one stop accepting
// connections at the same moment and drain what is in flight within the
// same bound, so the shutdown's budget counts the window once, not once per
// listener, and a client of one listener is not kept waiting for the
// other's drain. A stop that panics is reported the way a cleanup's panic
// is (see runSafe) and holds the others up no longer.
func stopTogether(abandon context.Context, stops ...func(context.Context)) {
	ctx, cancel := context.WithTimeout(abandon, lifecycle.StopTimeout)
	defer cancel()
	var wg sync.WaitGroup
	for _, stop := range stops {
		wg.Go(func() { runSafe(func() { stop(ctx) }) })
	}
	wg.Wait()
}

// closeComponent adapts a client's Close to a cleanup, logging the returned
// error centrally so shutdown always continues and the clients do not
// implement their own logging.
func closeComponent(name string, closeFn func() error) func() {
	return func() {
		if err := closeFn(); err != nil {
			zap.S().Errorw("failed to close component", "component", name, "err", err)
		}
	}
}
