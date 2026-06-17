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

// Exit will call all registered cleanup handlers and then exit.
func Exit(code int) {
	runHandlers()
	os.Exit(code)
}

// RegisterCleanup append custom cleanup handler, the handler will be invoked by `Cleanup` function.
// first handler will be called first
func RegisterCleanup(handler func()) {
	handlers = append(handlers, handler)
}

// DeferCleanup same as RegisterCleanup, but last handler will be called first.
func DeferCleanup(handler func()) {
	handlers = append([]func(){handler}, handlers...)
}

// Cleanup will call all registered cleanup handlers.
func Cleanup() {
	once.Do(runHandlers)
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
