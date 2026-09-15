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
// the renewals without releasing the lease; the holder releases once its
// work is done.
//
// On SQLite there is no renewal and no local deadline: the framework opens a
// single connection to it, and a renewal would wait behind the work it is
// meant to keep alive. The lease then holds for leaseDuration from the claim,
// and the context ends with parent alone.
func Hold(parent context.Context, h *Handle) (ctx context.Context, stop context.CancelFunc) {
	if db, _, err := primary(); err == nil && dialectOf(db) == "sqlite" {
		ctx, cancel := context.WithCancelCause(parent)
		return ctx, func() { cancel(nil) }
	}
	return hold(parent, h.name, h.Renew, renewInterval, localDeadline)
}

// hold is Hold with the renewal, the interval and the deadline injectable,
// so the loop is tested without a database and in milliseconds.
func hold(parent context.Context, name string, renew func(context.Context) error, interval, deadline time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	go func() {
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
			err := renew(attempt)
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
				zap.S().Warnw("lease renewal failed", "lease", name, "err", err)
			}
			if time.Since(last) >= deadline {
				cancel(ErrLost)
				return
			}
			timer.Reset(interval)
		}
	}()
	return ctx, func() { cancel(nil) }
}
