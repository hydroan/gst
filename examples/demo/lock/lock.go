// Package lock declares the application's locks: one for each piece of work
// that must not run twice at once across the deployment and is done when it
// returns — an administrator's "rebuild the report", a refresh of a
// credential every replica shares.
//
// Declare each lock in a package variable with lock.New(name) and try it
// from the code that triggers the work with TryRun(ctx, fn). The try never
// waits: a lock held elsewhere is refused at once with lock.ErrHeld, and the
// caller answers accordingly — a conflict to the client, a skipped run to
// the log. fn receives a context that ends when the lease behind the lock is
// lost or ctx ends, and must stop then — fn still running 5 seconds after
// the loss fails the process; a lost lease is reported as lock.ErrLost even
// when fn returned nothing, since another holder may have started the same
// work since. A lock protects a piece of work, not rows: two requests
// writing the same row are kept apart by a transaction and a row lock.
//
// On SQLite the framework uses a single database connection, so a
// transaction inside fn blocks the lease renewal: keep each transaction
// under 5 seconds, or the lease counts as lost.
//
// Example:
//
//	import (
//		"context"
//
//		"github.com/cockroachdb/errors"
//		"github.com/hydroan/gst/lock"
//	)
//
//	var rebuildReport = lock.New("rebuild-report")
//
//	func rebuild(ctx context.Context) error {
//		err := rebuildReport.TryRun(ctx, rebuildReportRows)
//		if errors.Is(err, lock.ErrHeld) {
//			return errors.New("a rebuild is already running")
//		}
//		return err
//	}
package lock

// TODO: declare your locks here, one package variable each.
