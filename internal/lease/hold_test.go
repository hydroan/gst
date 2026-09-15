package lease

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestHoldKeepsTheLeaseWhileRenewalsSucceed proves successful renewals keep
// moving the deadline: the held context outlives many deadlines' worth of
// time, the name stays refused to everyone else, and the context ends only
// when the holder stops the renewals — without ErrLost.
func TestHoldKeepsTheLeaseWhileRenewalsSucceed(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)

	held, stop := Hold(ctx, holder)
	select {
	case <-held.Done():
		t.Fatalf("the lease ended although every renewal succeeded: %v", context.Cause(held))
	case <-time.After(3 * localDeadline):
	}
	_, claimed, err = Claim(ctx, name)
	require.NoError(t, err)
	require.False(t, claimed, "a renewed lease is not free")

	stop()
	awaitDone(held, t)
	require.NotErrorIs(t, context.Cause(held), ErrLost, "stopping the renewals is not a loss")
	require.NoError(t, holder.Release(ctx))
}

// TestHoldEndsAtTheLocalDeadlineWithoutTheDatabase proves a holder that
// cannot reach the database gives itself up once the local deadline passes
// without a successful renewal — and not before: a single failure is not a
// loss. The database goes away by way of a closed connection handle.
func TestHoldEndsAtTheLocalDeadlineWithoutTheDatabase(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() { _ = holder.Release(ctx) })

	withClosedDatabase(t)
	begin := time.Now()
	held, stop := Hold(ctx, holder)
	defer stop()

	awaitDone(held, t)
	require.ErrorIs(t, context.Cause(held), ErrLost)
	require.GreaterOrEqual(t, time.Since(begin), localDeadline, "one failed renewal must not end the lease before the deadline")
}

// TestHoldGivesUpWhenARenewalCannotGetAConnection proves a renewal that
// cannot reach the database because every connection is taken — on SQLite
// the single connection, by a transaction of the work itself — is cut at
// the deadline instead of hanging, so the holder still gives itself up
// before its successor can start. The pool is narrowed to one connection
// and that one held by an open transaction for the whole hold.
func TestHoldGivesUpWhenARenewalCannotGetAConnection(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() { _ = holder.Release(ctx) })

	pool, err := dbruntime.DB.DB()
	require.NoError(t, err)
	limit := pool.Stats().MaxOpenConnections
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.SetMaxOpenConns(limit) })
	tx := dbruntime.DB.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })

	begin := time.Now()
	held, stop := Hold(ctx, holder)
	defer stop()

	awaitDone(held, t)
	require.ErrorIs(t, context.Cause(held), ErrLost)
	require.GreaterOrEqual(t, time.Since(begin), localDeadline, "a renewal kept waiting must not end the lease before the deadline")
}

// TestHoldEndsWithItsParent proves the held context ends with the context
// it derives from, so shutdown reaches the work under a lease.
func TestHoldEndsWithItsParent(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() { _ = holder.Release(ctx) })

	parent, cancelParent := context.WithCancel(ctx)
	held, stop := Hold(parent, holder)
	defer stop()

	cancelParent()
	awaitDone(held, t)
	require.ErrorIs(t, context.Cause(held), context.Canceled)
}

// TestHoldKeepsRenewingUntilStoppedAfterItsParentEnds proves the renewals
// outlive the parent context: parent ending tells the work to stop, and until
// the holder stops the renewals — once its work has returned — the name stays
// its own, so no other process starts the same work while this one is still
// winding down.
func TestHoldKeepsRenewingUntilStoppedAfterItsParentEnds(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)

	parent, cancelParent := context.WithCancel(ctx)
	held, stop := Hold(parent, holder)
	cancelParent()
	awaitDone(held, t)

	// Long past the lease's duration, the name is still refused to others.
	time.Sleep(2 * leaseDuration)
	_, claimed, err = Claim(ctx, name)
	require.NoError(t, err)
	require.False(t, claimed, "the lease must stay renewed until the holder stops the renewals")

	stop()
	require.NoError(t, holder.Release(ctx))
	_, claimed, err = Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed, "released once the work has returned, the name is free")
}

// TestHoldEndsWhenTheLeaseIsTakenAway proves the context Hold hands out
// ends with ErrLost once a renewal finds the lease gone, on every dialect.
func TestHoldEndsWhenTheLeaseIsTakenAway(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)

	held, stop := Hold(ctx, holder)
	defer stop()

	// An operator's hand: the lease is ended in the table behind the
	// holder's back.
	require.NoError(t, dbruntime.DB.Exec("UPDATE "+table+" SET expires_at_ms = 0 WHERE name = ?", name).Error)

	select {
	case <-held.Done():
		require.ErrorIs(t, context.Cause(held), ErrLost)
	case <-time.After(5 * time.Second):
		t.Fatal("the held context must end once the lease is gone")
	}
}

// TestRunReturnsWhatTheWorkReturned proves Run hands the work's outcome back
// as is, and turns a panic in the work into an error carrying its stack.
func TestRunReturnsWhatTheWorkReturned(t *testing.T) {
	ctx := context.Background()

	require.NoError(t, Run(ctx, "sample", func(context.Context) error { return nil }))
	require.ErrorContains(t, Run(ctx, "sample", func(context.Context) error { return errors.New("sample failure") }), "sample failure")
	err := Run(ctx, "sample", func(context.Context) error { panic("sample panic") })
	require.ErrorContains(t, err, "sample panic")
	require.Contains(t, fmt.Sprintf("%+v", err), "hold_test.go", "the error must carry the stack of the panic site")
}

// TestRunGivesLostWorkTheGraceToReturn proves a loss is not yet a failure:
// work that returns within the grace of losing its lease ends the run the
// ordinary way, and the process goes on.
func TestRunGivesLostWorkTheGraceToReturn(t *testing.T) {
	withFastProtocol(t)
	failures := withRecordedFailures(t)

	held, cancel := context.WithCancelCause(context.Background())
	cancel(ErrLost)
	require.ErrorIs(t, Run(held, "sample", context.Cause), ErrLost)
	require.Empty(t, failures, "work that returned in time must not fail the process")
}

// TestRunWaitsForTheWorkAtShutdown proves the context ending for any reason
// but a loss — the process shutting down — is not a failure: Run waits for
// the work for as long as it takes, and whoever stops the process bounds
// that wait.
func TestRunWaitsForTheWorkAtShutdown(t *testing.T) {
	withFastProtocol(t)
	failures := withRecordedFailures(t)

	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	returned := make(chan error, 1)
	go func() {
		returned <- Run(ctx, "sample", func(ctx context.Context) error {
			<-ctx.Done()
			<-release
			return ctx.Err()
		})
	}()

	cancel()
	select {
	case <-returned:
		t.Fatal("Run must wait for the work when the context ends without a loss")
	case <-time.After(3 * stepDownGrace):
	}
	close(release)
	require.ErrorIs(t, awaitReturned(t, returned), context.Canceled)
	require.Empty(t, failures, "a shutdown must not fail the process")
}

// TestRunFailsTheProcessWhenLostWorkWillNotStop proves the last line of the
// protocol: work still running once its lease is lost and the grace has
// passed fails the process, and Run keeps waiting for the work rather than
// handing control back beside it.
func TestRunFailsTheProcessWhenLostWorkWillNotStop(t *testing.T) {
	withFastProtocol(t)
	failures := withRecordedFailures(t)

	held, cancel := context.WithCancelCause(context.Background())
	release := make(chan struct{})
	returned := make(chan error, 1)
	go func() {
		returned <- Run(held, "sample", func(context.Context) error {
			<-release
			return nil
		})
	}()

	cancel(ErrLost)
	select {
	case err := <-failures:
		require.ErrorContains(t, err, `lease "sample" was lost and the work under it has not stopped`)
	case <-time.After(5 * time.Second):
		t.Fatal("work ignoring the loss must fail the process")
	}
	select {
	case <-returned:
		t.Fatal("Run must keep waiting for the work after failing the process")
	case <-time.After(3 * stepDownGrace):
	}
	close(release)
	require.NoError(t, awaitReturned(t, returned))
}

// TestInterruptedCountsOnlyTheContextsOwnEnding pins what work returning
// under an ended context counts as an interruption: the context's own
// error, as is or wrapped, and nothing that carries a failure of the work's
// own beside it.
func TestInterruptedCountsOnlyTheContextsOwnEnding(t *testing.T) {
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	failure := errors.New("sample failure")

	require.False(t, Interrupted(context.Background(), context.Canceled), "a context still running ended nothing")
	require.False(t, Interrupted(ended, nil), "work that returned nothing was not interrupted")
	require.True(t, Interrupted(ended, ended.Err()))
	require.True(t, Interrupted(ended, errors.Wrap(ended.Err(), "query")), "the ending wrapped is still the ending")
	require.True(t, Interrupted(ended, errors.Join(ended.Err(), errors.Wrap(ended.Err(), "twice"))), "the ending joined with itself is still the ending")
	require.False(t, Interrupted(ended, failure), "a failure of the work's own is not an interruption")
	require.False(t, Interrupted(ended, errors.Join(ended.Err(), failure)), "a failure beside the ending must not hide behind it")
	require.False(t, Interrupted(ended, errors.Wrap(errors.Join(ended.Err(), failure), "round")), "nor when the join is wrapped")
}

// withClosedDatabase points the engine at a connection handle whose pool is
// closed — a database that cannot be reached — and restores the suite's
// database afterwards. The handle still names its dialect, so the protocol
// gets as far as the statement, which then fails.
func withClosedDatabase(t *testing.T) {
	t.Helper()

	closed, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	pool, err := closed.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())

	original := dbruntime.DB
	dbruntime.DB = closed
	t.Cleanup(func() { dbruntime.DB = original })
}

// awaitDone waits for ctx to end, failing the test when it does not in time.
func awaitDone(ctx context.Context, t *testing.T) {
	t.Helper()

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the context did not end")
	}
}

// withRecordedFailures records the process failures Run reports instead of
// ending the test process, and restores the real one afterwards.
func withRecordedFailures(t *testing.T) <-chan error {
	t.Helper()

	failures := make(chan error, 4)
	t.Cleanup(SetFail(func(err error) { failures <- err }))
	return failures
}

// awaitReturned receives Run's return from returned, failing the test when it
// does not come in time.
func awaitReturned(t *testing.T, returned <-chan error) error {
	t.Helper()

	select {
	case err := <-returned:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
		return nil
	}
}
