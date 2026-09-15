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
// runs on that context: the transactions it opens end with it. stop ends
// the renewals and returns once they have — a renewal in flight is cut
// short — without releasing the lease; the holder releases once its work is
// done.
//
// The framework opens a single connection to SQLite, so there a renewal
// waits behind the work's own statements: a single transaction of the work
// that runs longer than localDeadline keeps the renewal from the connection
// and the lease counts as lost, the same as when the database cannot be
// reached.
func Hold(parent context.Context, h *Handle) (ctx context.Context, stop context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
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
			case <-ctx.Done():
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
			attempt, cancelAttempt := context.WithTimeout(ctx, min(interval, remaining))
			err := h.Renew(attempt)
			cancelAttempt()

			switch {
			case err == nil:
				last = started
			case errors.Is(err, ErrLost):
				cancel(ErrLost)
				return
			case ctx.Err() != nil:
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
		cancel(nil)
		<-done
	}
}
