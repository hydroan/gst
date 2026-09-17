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
// the loss fails the process, which exits without waiting for it; a lost
// lease is reported as lock.ErrLost even when fn returned nothing, since
// another holder may have started the same work since. A lock protects a
// piece of work, not rows: two requests writing the same row are kept apart
// by a transaction and a row lock.
//
// Try a lock outside any database transaction and open the transactions
// inside fn: the lock is given back as soon as fn returns, before a
// transaction around the try — a database.Transaction closure, a model
// hook — commits fn's writes, so such a try is refused with
// lock.ErrInTransaction.
//
// On SQLite the framework uses a single database connection, so a
// transaction inside fn blocks the lease renewal: keep each transaction
// under 8 seconds, and under 5 when transactions run back to back — a longer
// one may end the work with the lease counted as lost, one over 10 seconds
// always does.
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
