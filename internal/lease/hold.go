package lease

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
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
