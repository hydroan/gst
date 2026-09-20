package dao

import (
	"context"
	"time"

	"cluster/lock"

	"github.com/hydroan/gst/database"
)

// Rebuild runs the rebuild under its lock: it records the run, takes the given
// number of seconds, and marks the run ended. A rebuild already running — on
// this replica or another — makes the try return lock.ErrHeld at once; a lease
// lost while it runs ends it with lock.ErrLost, and its run keeps no end.
//
// inTransaction takes the lock from inside a transaction instead, which the
// framework refuses with lock.ErrInTransaction: the lock is given back when
// the work returns, before that transaction commits, so the next replica
// would start the work on rows this one has not written yet.
func Rebuild(ctx context.Context, seconds int, inTransaction bool) error {
	if inTransaction {
		return database.Transaction(ctx, func(ctx context.Context) error {
			return rebuildUnderLock(ctx, seconds)
		})
	}
	return rebuildUnderLock(ctx, seconds)
}

// rebuildUnderLock is the rebuild itself, taken under the lock.
func rebuildUnderLock(ctx context.Context, seconds int) error {
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
