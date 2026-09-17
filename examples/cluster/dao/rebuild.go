package dao

import (
	"context"
	"time"

	"cluster/lock"
)

// Rebuild runs the rebuild under its lock: it records the run, takes the given
// number of seconds, and marks the run ended. A rebuild already running — on
// this replica or another — makes the try return lock.ErrHeld at once; a lease
// lost while it runs ends it with lock.ErrLost, and its run keeps no end.
func Rebuild(ctx context.Context, seconds int) error {
	return lock.Rebuild.TryRun(ctx, func(ctx context.Context) error {
		run, err := StartRun(ctx, "lock", lock.Rebuild.Name())
		if err != nil {
			return err
		}
		select {
		case <-time.After(time.Duration(seconds) * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
		return EndRun(ctx, run)
	})
}
