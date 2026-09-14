package bootstrap

import (
	"fmt"
	"os"
	"slices"
	"sync"

	"go.uber.org/zap"
)

// The cleanup stack: whatever Bootstrap and Run bring up registers how it is
// torn down, and clean unwinds the stack once at shutdown.
var (
	cleanups  []func()
	cleanOnce sync.Once
)

// clean runs the cleanup stack once; later calls do nothing.
func clean() {
	cleanOnce.Do(runCleanups)
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
