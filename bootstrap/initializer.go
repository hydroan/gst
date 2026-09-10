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

var ins = new(initializer)

type initializer struct {
	fns []func() error // run init function in current goroutine.
	gos []func() error // long-running functions Go starts, each in its own goroutine.
}

func (i *initializer) Register(fn ...func() error) {
	i.fns = append(i.fns, fn...)
}

func (i *initializer) RegisterGo(fn ...func() error) {
	i.gos = append(i.gos, fn...)
}

// Init executes all registered initialization functions sequentially
// and logs their execution time for performance monitoring
func (i *initializer) Init() error {
	defer func() {
		i.fns = make([]func() error, 0)
	}()

	for j := range i.fns {
		fn := i.fns[j]
		if fn == nil {
			continue
		}

		// Execute function with timing measurement using defer pattern
		if err := i.executeWithTiming(fn); err != nil {
			return err
		}
	}
	return nil
}

// Go starts every registered long-running function in its own goroutine and
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
	defer func() {
		i.gos = make([]func() error, 0)
	}()

	g, failed := errgroup.WithContext(context.Background())
	for _, fn := range i.gos {
		if fn == nil {
			continue
		}
		g.Go(fn)
	}
	return failed
}

// executeWithTiming executes a function and logs its execution time
func (i *initializer) executeWithTiming(fn func() error) error {
	funcName := i.getFunctionName(fn)

	// Use defer pattern for cleaner timing measurement
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		// Log with structured fields for better observability
		zap.S().Debugw("Init function executed", "function", funcName, util.LogDuration(duration))
	}()

	return fn()
}

// getFunctionName extracts a clean function name from function pointer
func (i *initializer) getFunctionName(fn func() error) string {
	if fn == nil {
		return "<nil>"
	}

	pc := runtime.FuncForPC(reflect.ValueOf(fn).Pointer())
	if pc == nil {
		return "<unknown>"
	}

	fullName := pc.Name()
	// Extract package.function from full path for cleaner logs
	if lastSlash := strings.LastIndex(fullName, "/"); lastSlash >= 0 {
		fullName = fullName[lastSlash+1:]
	}

	return fullName
}
