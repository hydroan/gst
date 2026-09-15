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

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Register declares fn as the work named name, run on every replica for the
// life of the process. Registration belongs in package init functions: the
// process starts the work in Run, after the routes-ready hooks — every table
// exists and is seeded — and before the listener opens, on a context that
// ends the moment shutdown begins. fn returns once that context has ended
// and its work is done; shutdown waits for it, up to the bound the process
// gives its components, and the providers close after it. Work the
// deployment must do once belongs to cronjob, leader or lock, not here.
//
// fn runs until the process stops: returning before its context has ended
// is a failure, nil included — a loop that quietly ends would leave the
// process running without it — and so is a panic. Each ends the process the
// way a failing listener does, with the reason, for the orchestrator to
// restart it. A nil fn, an empty name, a name already registered, or a
// registration after the process started its components is a programmer
// error and panics.
func Register(fn func(ctx context.Context) error, name string) {
	if fn == nil {
		panic(fmt.Sprintf("component: register requires a non-nil function for %q", name))
	}
	w := &work{name: name, fn: fn}
	lifecycle.Register(lifecycle.Component{
		Name:  name,
		Stage: lifecycle.StageComponent,
		Start: w.start,
		Stop:  w.stop,
	})
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

// start runs fn on a goroutine of its own, on ctx — the process context,
// which ends when shutdown begins — and returns at once.
func (w *work) start(ctx context.Context) error {
	w.done = make(chan struct{})
	go w.run(ctx)
	return nil
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
	if err != nil && !util.Interrupted(ctx, err) {
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

// stop waits for fn to return, for as long as ctx allows: the process
// context fn runs on has ended by now, and returning is what is asked of it.
func (w *work) stop(ctx context.Context) error {
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return errors.Newf("component %q has not returned", w.name)
	}
}
