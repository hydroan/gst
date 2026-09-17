package lock_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/cronjob"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/lock"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/service"
)

// rebuildReport is declared the way a project declares a lock: once, in a
// package variable of its lock package.
var rebuildReport = lock.New("rebuild-report")

// ExampleNew declares a lock. Declarations belong in package variables: the
// locks are checked as the process starts — every name one the lease table can
// hold, no name declared twice, a primary database that can carry a lease —
// and a declaration made after that panics. So a lock is never declared where
// its work is triggered:
//
//	// Panics once the process has started.
//	err := lock.New("rebuild-report").TryRun(ctx, rebuildReportRows)
func ExampleNew() {
	fmt.Println(rebuildReport.Name())
	// Output: rebuild-report
}

// ExampleLock_TryRun runs work a request triggers under its lock. The try
// never waits: while the work runs elsewhere — on another replica, or for
// another request on this one — it is refused at once with lock.ErrHeld,
// which the service answers with a conflict. The work opens its transactions
// inside, on the context it receives, and they check the lease before their
// first statement. In a project the try sits in a dao function, and the
// service method maps what it returns.
func ExampleLock_TryRun() {
	var ctx context.Context // the request's, in the service method

	err := rebuildReport.TryRun(ctx, func(ctx context.Context) error {
		return database.Transaction(ctx, rebuildReportRows)
	})
	switch {
	case errors.Is(err, lock.ErrHeld):
		err = service.NewError(http.StatusConflict, "a rebuild is already running")
	case errors.Is(err, lock.ErrLost):
		err = service.NewError(http.StatusConflict, "the rebuild was cut short, try again")
	}
	_ = err // returned by the service method
}

// ExampleLock_TryRun_scheduledJob hands long work from a request to a
// scheduled job. The work runs on the context the try is given, so work tried
// in a request stops when the client disconnects or the request times out. A
// request asking for a long rebuild records the ask instead, and a job picks
// it up. The same lock keeps the job apart from a rebuild an administrator
// starts by hand, and a job that finds the lock held skips the round: the
// other holder is doing the work already.
func ExampleLock_TryRun_scheduledJob() {
	// cronjob/cronjob.go, from init:
	cronjob.Register(func(ctx context.Context) error {
		requested, err := reportRebuildRequested(ctx)
		if err != nil || !requested {
			return err
		}
		err = rebuildReport.TryRun(ctx, func(ctx context.Context) error {
			return database.Transaction(ctx, func(ctx context.Context) error {
				if rebuildErr := rebuildReportRows(ctx); rebuildErr != nil {
					return rebuildErr
				}
				return clearReportRebuildRequest(ctx)
			})
		})
		if errors.Is(err, lock.ErrHeld) {
			return nil
		}
		return err
	}, "@every 1m", "report-rebuild")
}

// ExampleLock_TryRun_insideTransaction shows the try an open transaction
// refuses. The lock is given back as soon as the work returns, while a
// transaction around the try — a database.Transaction closure, or the write a
// model hook runs in — commits only after that: another holder could start
// the same work before this one's writes show. So a try made inside an open
// transaction is refused with lock.ErrInTransaction, before it claims
// anything. Take the lock first and open the transaction inside the work; a
// try that follows writes of a transaction is made once it has committed.
func ExampleLock_TryRun_insideTransaction() {
	var ctx context.Context // the caller's

	// Refused: errors.Is(err, lock.ErrInTransaction) reports true.
	err := database.Transaction(ctx, func(ctx context.Context) error {
		return rebuildReport.TryRun(ctx, rebuildReportRows)
	})
	_ = err

	// The lock first, the transaction inside the work.
	err = rebuildReport.TryRun(ctx, func(ctx context.Context) error {
		return database.Transaction(ctx, rebuildReportRows)
	})
	_ = err

	// Writes of its own first, then the try, once they have committed.
	err = database.Transaction(ctx, func(ctx context.Context) error {
		if requestErr := requestReportRebuild(ctx); requestErr != nil {
			return requestErr
		}
		return database.AfterCommit(ctx, func(ctx context.Context) error {
			return rebuildReport.TryRun(ctx, rebuildReportRows)
		})
	})
	_ = err
}

// ExampleLock_TryRun_leaseLost stops work whose lease is lost. The work's
// context ends when the lease is lost — the replica could not renew it for 10
// seconds, or found it taken — and the work must stop then: work still running
// 5 seconds later fails the process, which exits without waiting for it, since
// another holder may be running the same work by then. The try reports
// lock.ErrLost even when the work returned nothing, joined with the work's own
// error when it returned one: another holder may have started the work since,
// so what this run did is not the whole story.
//
// On SQLite, where the framework opens a single connection, a transaction of
// the work holds the lease renewal back: keep each under 8 seconds, and under
// 5 when they run back to back.
func ExampleLock_TryRun_leaseLost() {
	var ctx context.Context // the caller's

	err := rebuildReport.TryRun(ctx, func(ctx context.Context) error {
		sections, err := reportSections(ctx)
		if err != nil {
			return err
		}
		for _, section := range sections {
			// Stop between steps once the context has ended.
			if err := ctx.Err(); err != nil {
				return err
			}
			err := database.Transaction(ctx, func(ctx context.Context) error {
				return rebuildReportSection(ctx, section)
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, lock.ErrLost) {
		// The rebuild may be half done, and another holder may be redoing it:
		// report it as not done.
		logger.App.Warnw("report rebuild cut short", "err", err)
	}
}

// ExampleLock_TryRun_onceOnly keeps a result from happening twice. A lock only
// keeps two runs of the work from overlapping: once it is given back, the next
// try — on this replica or another — runs the work again, and a run cut short
// by a lost lease may have done part of it. What must happen once per record is
// settled by the database instead: the work claims the record with an update
// conditioned on its state, and leaves a record an earlier run claimed alone.
func ExampleLock_TryRun_onceOnly() {
	var ctx context.Context // the caller's
	var recordID int64      // the record the notice is for

	err := recordNotices.TryRun(ctx, func(ctx context.Context) error {
		var claimed bool
		err := database.Transaction(ctx, func(ctx context.Context) error {
			var err error
			// UPDATE ... SET notice_state = 'sending'
			//  WHERE id = :id AND notice_state = 'pending'
			claimed, err = claimRecordNotice(ctx, recordID)
			return err
		})
		if err != nil || !claimed {
			return err
		}
		return sendRecordNotice(ctx, recordID)
	})
	_ = err
}

// The declarations below stand in for the project's own code.

// recordNotices is the lock the notices of records are sent under.
var recordNotices = lock.New("record-notices")

// rebuildReportRows rebuilds the rows of the report.
func rebuildReportRows(context.Context) error { return nil }

// reportSections lists the sections the report is rebuilt in.
func reportSections(context.Context) ([]string, error) { return nil, nil }

// rebuildReportSection rebuilds one section of the report.
func rebuildReportSection(context.Context, string) error { return nil }

// requestReportRebuild records that a rebuild of the report was asked for.
func requestReportRebuild(context.Context) error { return nil }

// reportRebuildRequested reports whether a rebuild of the report was asked for.
func reportRebuildRequested(context.Context) (bool, error) { return false, nil }

// clearReportRebuildRequest removes the ask for a rebuild.
func clearReportRebuildRequest(context.Context) error { return nil }

// claimRecordNotice moves a record's notice from pending to sending, and
// reports whether this call moved it.
func claimRecordNotice(context.Context, int64) (bool, error) { return false, nil }

// sendRecordNotice sends the notice of a record.
func sendRecordNotice(context.Context, int64) error { return nil }
