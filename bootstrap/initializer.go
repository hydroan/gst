package bootstrap

import (
	"context"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// startup sequences the process: Bootstrap feeds it the init functions of
// each layer and runs them in turn, Run feeds it the listeners and starts
// them side by side.
var startup = new(initializer)

// initializer holds what has been registered and not yet run. Init and Go
// each empty their queue as they run it, so the next Register starts a new
// round.
type initializer struct {
	fns []func() error // init functions; Init runs them one after another in the calling goroutine.
	gos []func() error // long-running functions; Go starts each in a goroutine of its own.
}

// Register queues init functions for the next Init, in the order they run.
func (i *initializer) Register(fn ...func() error) {
	i.fns = append(i.fns, fn...)
}

// RegisterGo queues long-running functions for the next Go.
func (i *initializer) RegisterGo(fn ...func() error) {
	i.gos = append(i.gos, fn...)
}

// Init runs the queued init functions one after another in registration
// order, logging how long each took, and stops at the first failure, which
// it returns.
func (i *initializer) Init() error {
	defer func() { i.fns = nil }()

	for _, fn := range i.fns {
		if fn == nil {
			continue
		}
		if err := runTimed(fn); err != nil {
			return err
		}
	}
	return nil
}

// Go starts every queued long-running function in its own goroutine and
// returns at once, with a context that is canceled the moment any of them
// returns an error; that error is the context's cause.
//
// It does not wait for the functions to finish. They are servers that block
// until shut down, so waiting for all of them would hold one's failure back
// for as long as any other keeps serving: a listener failing to start beside
// one that stays up would leave the process running with nothing to report.
// Shutting down the ones still serving is not done here either; the caller
// returns once the context is canceled, and its cleanup stops them.
//
// A function that returns nil has finished without failing — a listener that
// is not enabled returns at once — and cancels nothing.
func (i *initializer) Go() context.Context {
	defer func() { i.gos = nil }()

	g, failed := errgroup.WithContext(context.Background())
	for _, fn := range i.gos {
		if fn == nil {
			continue
		}
		g.Go(fn)
	}
	return failed
}

// runTimed runs fn and logs how long it took, under its name.
func runTimed(fn func() error) error {
	name := functionName(fn)

	begin := time.Now()
	defer func() {
		zap.S().Debugw("Init function executed", "function", name, util.LogDuration(time.Since(begin)))
	}()

	return fn()
}

// functionName names fn the way the timing log refers to it: package and
// function, without the import path.
func functionName(fn func() error) string {
	if fn == nil {
		return "<nil>"
	}

	pc := runtime.FuncForPC(reflect.ValueOf(fn).Pointer())
	if pc == nil {
		return "<unknown>"
	}

	name := pc.Name()
	if lastSlash := strings.LastIndex(name, "/"); lastSlash >= 0 {
		name = name[lastSlash+1:]
	}
	return name
}
