package bootstrap

import (
	"fmt"
	"os"
	"slices"
	"sync"
)

var (
	handlers = []func(){}
	once     sync.Once
)

// clean will call all registered clean handlers.
func clean() {
	once.Do(runHandlers)
}

// registerCleanup appends a cleanup handler. Handlers run serially in
// reverse registration order (LIFO) when the process shuts down: register
// dependencies first and dependents later, exactly like defer.
func registerCleanup(handler func()) {
	handlers = append(handlers, handler)
}

// runHandlers runs the cleanup handlers serially in reverse registration
// order: teardown mirrors setup the way a defer stack unwinds, so whatever
// was brought up last (the HTTP listener draining in-flight requests) is
// torn down first, and what everything else depends on (connections, log
// writers, the temp directory) is torn down after the drain finished.
func runHandlers() {
	for _, handler := range slices.Backward(handlers) {
		runSafe(handler)
	}
}

func runSafe(handler func()) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup handler error:", err)
		}
	}()

	handler()
}
