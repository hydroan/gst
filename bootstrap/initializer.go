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

var _initializer = new(initializer)

type initializer struct {
	fns []func() error // run init function in current goroutine.
	gos []func() error // run init function in new goroutine and receive error in channel.
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

func (i *initializer) Go() error {
	defer func() {
		i.gos = make([]func() error, 0)
	}()

	g, _ := errgroup.WithContext(context.Background())
	for _, fn := range i.gos {
		if fn == nil {
			continue
		}
		g.Go(fn)

	}
	return g.Wait()
}

// executeWithTiming executes a function and logs its execution time
func (i *initializer) executeWithTiming(fn func() error) error {
	funcName := i.getFunctionName(fn)

	// Use defer pattern for cleaner timing measurement
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		// Log with structured fields for better observability
		zap.S().Debugw("Init function executed", "function", funcName, "cost", util.FormatDurationSmart(duration))
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

func Register(fn ...func() error)   { _initializer.Register(fn...) }
func RegisterGo(fn ...func() error) { _initializer.RegisterGo(fn...) }
func Init() (err error)             { return _initializer.Init() }
func Go() (err error)               { return _initializer.Go() }
