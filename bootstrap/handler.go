package bootstrap

import (
	"context"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sync/errgroup"
)

var (
	handlers = []func(){}
	once     sync.Once
)

// exit will call all registered cleanup handlers and then exit.
//
//nolint:unused
func exit(code int) {
	runHandlers()
	os.Exit(code)
}

// clean will call all registered clean handlers.
func clean() {
	once.Do(runHandlers)
}

// registerCleanup append custom cleanup handler, the handler will be invoked by `Cleanup` function.
// first handler will be called first
func registerCleanup(handler func()) {
	handlers = append(handlers, handler)
}

// deferCleanup same as RegisterCleanup, but last handler will be called first.
//
//nolint:unused
func deferCleanup(handler func()) {
	handlers = append([]func(){handler}, handlers...)
}

func runHandlers() {
	g, _ := errgroup.WithContext(context.Background())
	for _, handler := range handlers {
		g.Go(func() error { runSafe(handler); return nil })
	}
	_ = g.Wait()
}

func runSafe(handler func()) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup handler error:", err)
		}
	}()

	handler()
}
