// Package component runs a project's own long-running work alongside the
// server — a consumer loop, a poller, a watcher: work that runs on every
// replica for the life of the process. Register declares it; the process
// starts it once every table exists and is seeded, right before the
// listener opens, and stops it first at shutdown, before the providers it
// may use close.
package component

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

var (
	mu sync.Mutex
	// works holds every registration, in registration order.
	works []*work
	// errRegister collects the registrations that cannot be honored; start
	// fails on it, so a bad registration is reported at startup and not
	// lost.
	errRegister error
	// started is set by start: a registration after that would never run,
	// so it fails fast instead.
	started bool
)

func init() {
	// Importing this package is what enables the work: through the
	// lifecycle registry, bootstrap starts every registered function once
	// the tables are ready and stops them as soon as the process begins to
	// drain. A project that registers nothing never imports the package and
	// never starts anything.
	lifecycle.Register(lifecycle.Component{
		Name:  "component",
		Stage: lifecycle.StageComponent,
		Start: start,
		Stop:  stop,
	})
}

// Register declares fn as the work named name, run on every replica for the
// life of the process. Registration belongs in package init functions: the
// process starts the work in Run, after the routes-ready hooks — every table
// exists and is seeded — and before the listener opens, on a context that
// ends the moment shutdown begins. fn returns once that context has ended
// and its work is done; shutdown waits for it, up to the bound the process
// gives its components — work that outlives the bound is left behind as the
// process ends. Work the deployment must do once belongs to cronjob, leader
// or lock, not here.
//
// fn runs until the process stops: returning before its context has ended
// is a failure, nil included — a loop that quietly ends would leave the
// process running without it — and so is a panic. Each ends the process the
// way a failing listener does, with the reason, for the orchestrator to
// restart it.
//
// A registration that cannot be honored — no name, a nil fn, a name already
// registered — fails the process at startup; names are the package's own,
// so they never collide with a framework component's. A registration after
// the process started its components panics: it would never run.
func Register(fn func(ctx context.Context) error, name string) {
	mu.Lock()
	defer mu.Unlock()

	if started {
		panic(fmt.Sprintf("component: %q registered after the components started; register components in package init functions", name))
	}
	w, err := newWork(fn, name)
	if err != nil {
		errRegister = errors.Join(errRegister, err)
		return
	}
	works = append(works, w)
}

// newWork validates one registration. The caller holds mu: the duplicate
// check reads works.
func newWork(fn func(ctx context.Context) error, name string) (*work, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return nil, errors.New("component: registered work has no name")
	case fn == nil:
		return nil, errors.Newf("component %q: nil function", name)
	case slices.ContainsFunc(works, func(w *work) bool { return w.name == name }):
		return nil, errors.Newf("component %q: registered twice", name)
	}
	return &work{name: name, fn: fn}, nil
}

// start runs every registered work on ctx — the process context, which ends
// when shutdown begins — and returns at once. A registration that could not
// be honored fails the start.
func start(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if errRegister != nil {
		return errRegister
	}
	started = true
	for _, w := range works {
		w.start(ctx)
	}
	return nil
}

// stop waits for every work to return, for as long as ctx allows: the
// process context the work runs on has ended by now, and returning is what
// is asked of it. Work that has not returned when ctx ends is reported and
// left behind.
func stop(ctx context.Context) error {
	mu.Lock()
	running := works
	mu.Unlock()

	var err error
	for _, w := range running {
		err = errors.Join(err, w.stop(ctx))
	}
	return err
}

// fail ends the process when work fails; a test observes the call instead.
var fail = lifecycle.Fail

// work is one registered function and the goroutine it runs on.
type work struct {
	name string
	fn   func(ctx context.Context) error
	// done closes once fn has returned.
	done chan struct{}
}

// start runs fn on a goroutine of its own, on ctx.
func (w *work) start(ctx context.Context) {
	w.done = make(chan struct{})
	go w.run(ctx)
}

// run calls fn and reports how it ended. Before the process began to stop
// any return is a failure; after, returning is what was asked of fn, and a
// failure of its own then is logged, since the process ends either way.
func (w *work) run(ctx context.Context) {
	defer close(w.done)
	err := w.call(ctx)
	if ctx.Err() == nil {
		if err == nil {
			err = errors.Newf("component %q returned before the process began to stop", w.name)
		} else {
			err = errors.Wrapf(err, "component %q failed", w.name)
		}
		fail(err)
		return
	}
	if err != nil && !lifecycle.Interrupted(ctx, err) {
		zap.S().Warnw("component ended with an error while stopping", "component", w.name, "err", err)
	}
}

// call runs fn and turns a panic in it into an error carrying the stack of
// the panic site.
func (w *work) call(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = util.PanicError(r)
		}
	}()
	return w.fn(ctx)
}

// stop waits for fn to return, for as long as ctx allows.
func (w *work) stop(ctx context.Context) error {
	if !lifecycle.Await(ctx, w.done) {
		return errors.Newf("component %q has not returned", w.name)
	}
	return nil
}
