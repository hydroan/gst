package lease

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/util"
)

// Hold keeps h renewed on a goroutine of its own and returns a context that
// ends the moment the lease is known to be lost — a renewal reports it gone,
// or none succeeds within localDeadline of the start of the last one that
// did — with ErrLost as the cause; parent ending ends it too. The work under
// the lease runs on that context: the transactions it opens end with it.
//
// A renewal is due renewInterval after the previous attempt started, the
// claim counting as the first, and a renewal that failed is tried again at
// the same cadence, the way client-go renews: a database that drops a
// statement or stalls for a few seconds costs the holder nothing. Each
// attempt waits at most half of localDeadline — the bound client-go gives
// each of its requests — and never past the deadline, so an attempt that
// hangs leaves room for another and cannot carry the holder past it.
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
// shorter than localDeadline minus renewInterval can never do that on its
// own, and one longer than localDeadline always does. Transactions back to
// back leave the renewals no turn of their own: a renewal that waited out one
// transaction succeeds, and the attempt due right after it waits out the
// next, bounded by half of localDeadline — so each transaction must be
// shorter than that.
//
// log is the holder's own logger: a renewal that failed without losing the
// lease is what explains a round or a tenure cut short, so the entry belongs
// beside the holder's own rather than in the global stream.
func Hold(parent context.Context, h *Handle, log types.Logger) (ctx context.Context, stop context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	// The renewals run on a context of their own, so that parent ending does
	// not end them; stop does.
	renewing, stopRenewing := context.WithCancel(context.WithoutCancel(parent))
	interval, deadline := renewInterval, localDeadline
	// lose records the loss on the handle before it ends ctx, so whoever sees
	// ctx end with ErrLost finds the handle lost as well.
	lose := func() {
		h.markLost()
		cancel(ErrLost)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The claim counts as the last attempt and the last successful
		// renewal, from the moment it was sent.
		attempted, last := h.claimedAt, h.claimedAt
		timer := time.NewTimer(untilNextRenewal(attempted, last, interval, deadline))
		defer timer.Stop()
		for {
			select {
			case <-renewing.Done():
				return
			case <-timer.C:
			}

			attempted = time.Now()
			remaining := last.Add(deadline).Sub(attempted)
			if remaining <= 0 {
				lose()
				return
			}
			attempt, cancelAttempt := context.WithTimeout(renewing, min(deadline/2, remaining))
			err := h.Renew(attempt)
			cancelAttempt()

			switch {
			case err == nil:
				last = attempted
			case errors.Is(err, ErrLost):
				lose()
				return
			case renewing.Err() != nil:
				return
			default:
				log.Warnw("lease renewal failed", "component", "lease", "lease", h.name, "err", err)
			}
			if time.Since(last) >= deadline {
				lose()
				return
			}
			timer.Reset(untilNextRenewal(attempted, last, interval, deadline))
		}
	}()
	return ctx, func() {
		stopRenewing()
		cancel(nil)
		<-done
	}
}

// untilNextRenewal returns how long a holder waits before its next renewal
// attempt: interval after the previous attempt started — at once when an
// attempt took longer — but never past the deadline, deadline after the last
// successful renewal started. An attempt that failed just short of the
// deadline must not leave the holder asleep past it, on the other side of
// the margin its successor's claim respects.
func untilNextRenewal(attempted, last time.Time, interval, deadline time.Duration) time.Duration {
	return max(min(time.Until(attempted.Add(interval)), time.Until(last.Add(deadline))), 0)
}

// fail ends the process, without waiting for anything still running, when
// work under a lost lease will not stop; a test observes the call instead.
var fail = lifecycle.FailNow

// SetFail replaces what Run does once work under a lost lease will not stop,
// and returns the function that restores it. It exists for the tests of the
// capabilities built on leases, which record the failure instead of ending
// the test process — the process-wide failure is one-way, so a test that
// tripped it could not run twice; nothing else calls it.
func SetFail(fn func(error)) (restore func()) {
	original := fail
	fail = fn
	return func() { fail = original }
}

// Run runs work on ctx — the context Hold returned, or one derived from it —
// and returns what work returned. It is the protocol's last line. The
// deadline stops the renewals, the context ends the work and Verify refuses
// its next transaction; work that ignores all three and runs on once the
// lease is lost would run beside its successor's, the very thing the lease
// exists to rule out. So once ctx ends with ErrLost the work has
// stepDownGrace to return, and past that the process fails through
// lifecycle.FailNow — bootstrap ends Run without waiting for anything still
// running, this work included, and the orchestrator restarts the replica —
// while the wait goes on for as long as the process lasts, so the caller
// never claims again beside work of its own still running. ctx ending
// for any other reason — the process shutting down — is not a loss: Run
// waits for the work, and whoever stops the process bounds that wait. The
// renewals go on while the work winds down, though, and a lease they find
// lost meanwhile — h is lost, see Handle.Lost — leaves the work the grace
// like any other loss.
//
// The work runs on a goroutine of its own, so that the lease being lost can
// be watched while it runs; a panic in it is recovered into an error
// carrying the stack of the panic site, which Run returns.
//
// h is the handle the work runs under: its name is what a failure names.
// log is the holder's own logger: work that will not stop is the last thing
// said about a round or a tenure, so the entry belongs beside the holder's
// own rather than in the global stream.
func Run(ctx context.Context, h *Handle, log types.Logger, work func(ctx context.Context) error) error {
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
	case <-h.lost:
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), ErrLost) {
			select {
			case err := <-returned:
				return err
			case <-h.lost:
			}
		}
	}

	grace := time.NewTimer(stepDownGrace)
	defer grace.Stop()
	select {
	case err := <-returned:
		return err
	case <-grace.C:
		err := errors.Newf("lease %q was lost and the work under it has not stopped within %s", h.name, stepDownGrace)
		log.Errorw("work under a lost lease will not stop", "component", "lease", "lease", h.name, "err", err)
		fail(err)
		return <-returned
	}
}
