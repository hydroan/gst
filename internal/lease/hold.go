package lease

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// Hold keeps h renewed on a goroutine of its own and returns a context that
// ends the moment the lease is known to be lost — a renewal reports it gone,
// or none succeeds within localDeadline of the last one that did — with
// ErrLost as the cause; parent ending ends it too. The work under the lease
// runs on that context: the transactions it opens end with it.
//
// The renewals outlive parent on purpose. Parent ending tells the work to
// stop, and until it has — a round or a tenure winding down, a transaction
// rolling back — the name must stay this holder's, or another process could
// start the same work while it is still in flight. stop ends the renewals
// and returns once they have — a renewal in flight is cut short — without
// releasing the lease; the holder calls it once its work has returned, then
// releases.
//
// The framework opens a single connection to SQLite, so there a renewal
// waits behind the work's own statements: a transaction of the work still
// open when a renewal falls due keeps the renewal from the connection, and
// once localDeadline has passed since the last renewal the lease counts as
// lost, the same as when the database cannot be reached. A transaction
// shorter than localDeadline minus renewInterval can never do that; one
// longer than localDeadline always does.
func Hold(parent context.Context, h *Handle) (ctx context.Context, stop context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	// The renewals run on a context of their own, so that parent ending does
	// not end them; stop does.
	renewing, stopRenewing := context.WithCancel(context.WithoutCancel(parent))
	interval, deadline := renewInterval, localDeadline
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The claim counts as the last successful renewal.
		last := time.Now()
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-renewing.Done():
				return
			case <-timer.C:
			}

			// A renewal is bounded by the time left until the deadline, so
			// one that hangs cannot carry the holder past it.
			started := time.Now()
			remaining := time.Until(last.Add(deadline))
			if remaining <= 0 {
				cancel(ErrLost)
				return
			}
			attempt, cancelAttempt := context.WithTimeout(renewing, min(interval, remaining))
			err := h.Renew(attempt)
			cancelAttempt()

			switch {
			case err == nil:
				last = started
			case errors.Is(err, ErrLost):
				cancel(ErrLost)
				return
			case renewing.Err() != nil:
				return
			default:
				zap.S().Warnw("lease renewal failed", "lease", h.name, "err", err)
			}
			if time.Since(last) >= deadline {
				cancel(ErrLost)
				return
			}
			timer.Reset(interval)
		}
	}()
	return ctx, func() {
		stopRenewing()
		cancel(nil)
		<-done
	}
}

// fail ends the process when work under a lost lease will not stop; a test
// observes the call instead.
var fail = lifecycle.Fail

// Interrupted reports whether err is nothing but ctx's ending: the work
// returned the cancellation of the context it ran on — as is, or wrapped —
// which is what work is asked to do when its lease is lost or the process
// shuts down, not a failure of its own. An error that also carries a failure
// of the work's own, such as a join of the cancellation with another error,
// is not an interruption: the failure must not hide behind the ending.
func Interrupted(ctx context.Context, err error) bool {
	if ctx.Err() == nil || err == nil {
		return false
	}
	return onlyCause(err, ctx.Err())
}

// onlyCause reports whether every leaf of err's tree is target: a wrapper
// has one leaf, its cause; a join has one leaf per member.
func onlyCause(err, target error) bool {
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		members := multi.Unwrap()
		if len(members) == 0 {
			return false
		}
		for _, member := range members {
			if !onlyCause(member, target) {
				return false
			}
		}
		return true
	}
	if next := errors.Unwrap(err); next != nil {
		return onlyCause(next, target)
	}
	return errors.Is(err, target)
}

// Run runs work on ctx — the context Hold returned, or one derived from it —
// and returns what work returned. It is the protocol's last line. The
// deadline stops the renewals, the context ends the work and Verify refuses
// its next transaction; work that ignores all three and runs on once the
// lease is lost would run beside its successor's, the very thing the lease
// exists to rule out. So once ctx ends with ErrLost the work has
// stepDownGrace to return, and past that the process fails through
// lifecycle.Fail — bootstrap ends its Run, the orchestrator restarts the
// replica — while the wait goes on for as long as the process lasts, so the
// caller never claims again beside work of its own still running. ctx ending
// for any other reason — the process shutting down — is not a loss: Run
// waits for the work, and whoever stops the process bounds that wait.
//
// The work runs on a goroutine of its own, so that the lease being lost can
// be watched while it runs; a panic in it is recovered into an error
// carrying the stack of the panic site, which Run returns.
func Run(ctx context.Context, name string, work func(ctx context.Context) error) error {
	returned := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				returned <- util.PanicError(r)
			}
		}()
		returned <- work(ctx)
	}()

	select {
	case err := <-returned:
		return err
	case <-ctx.Done():
	}
	if !errors.Is(context.Cause(ctx), ErrLost) {
		return <-returned
	}

	grace := time.NewTimer(stepDownGrace)
	defer grace.Stop()
	select {
	case err := <-returned:
		return err
	case <-grace.C:
		err := errors.Newf("lease %q was lost and the work under it has not stopped within %s", name, stepDownGrace)
		zap.S().Errorw("work under a lost lease will not stop", "lease", name, "err", err)
		fail(err)
		return <-returned
	}
}
