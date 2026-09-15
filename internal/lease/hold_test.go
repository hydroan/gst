package lease

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/types"
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

	held, stop := Hold(ctx, holder, newHolderLog())
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

	// The deadline runs from the claim, so the measure starts before it.
	begin := time.Now()
	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(func() { _ = holder.Release(ctx) })

	withClosedDatabase(t)
	held, stop := Hold(ctx, holder, newHolderLog())
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

	// The deadline runs from the claim, so the measure starts before it.
	begin := time.Now()
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

	held, stop := Hold(ctx, holder, newHolderLog())
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
	held, stop := Hold(parent, holder, newHolderLog())
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
	withTolerantProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)

	parent, cancelParent := context.WithCancel(ctx)
	held, stop := Hold(parent, holder, newHolderLog())
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

	held, stop := Hold(ctx, holder, newHolderLog())
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

// sampleHandle is a handle for work run outside the claim protocol: a name
// for the failure to report, claimed just now.
func sampleHandle() *Handle {
	return &Handle{name: "sample", holder: "sample", claimedAt: time.Now()}
}

// TestRunReturnsWhatTheWorkReturned proves Run hands the work's outcome back
// as is, and turns a panic in the work into an error carrying its stack.
func TestRunReturnsWhatTheWorkReturned(t *testing.T) {
	ctx := context.Background()

	require.NoError(t, Run(ctx, sampleHandle(), newHolderLog(), func(context.Context) error { return nil }))
	require.ErrorContains(t, Run(ctx, sampleHandle(), newHolderLog(), func(context.Context) error { return errors.New("sample failure") }), "sample failure")
	err := Run(ctx, sampleHandle(), newHolderLog(), func(context.Context) error { panic("sample panic") })
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
	require.ErrorIs(t, Run(held, sampleHandle(), newHolderLog(), context.Cause), ErrLost)
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
		returned <- Run(ctx, sampleHandle(), newHolderLog(), func(ctx context.Context) error {
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
		returned <- Run(held, sampleHandle(), newHolderLog(), func(context.Context) error {
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

// TestHoldAndRunLogThroughTheHoldersLogger proves both entries the protocol
// writes go to the logger the holder passed in — the cron job's, the
// leader's, the lock's — and not to the global stream: a renewal that failed
// is what explains a round cut short, and work that will not stop is the last
// thing said about it, so both belong in the file the rest of that work logs
// to. The entries still name the protocol, so a reader can tell them from the
// holder's own.
func TestHoldAndRunLogThroughTheHoldersLogger(t *testing.T) {
	withFastProtocol(t)
	// The failure the stubborn work below trips is recorded, not acted on;
	// this test is about the entry that goes with it.
	withRecordedFailures(t)
	entries := newHolderLog()

	// A renewal that cannot reach the database fails without losing the
	// lease, which is what the warning reports.
	withClosedDatabase(t)
	h := sampleHandle()
	held, stop := Hold(context.Background(), h, entries)
	defer stop()

	renewal := entries.await(t, "lease renewal failed")
	require.Equal(t, "lease", renewal["component"])
	require.Equal(t, "sample", renewal["lease"])
	require.NotNil(t, renewal["err"])

	// The hold above ends with ErrLost once the deadline passes, and work
	// that ignores the loss is what the failure reports.
	release := make(chan struct{})
	returned := make(chan error, 1)
	go func() {
		returned <- Run(held, h, entries, func(context.Context) error {
			<-release
			return nil
		})
	}()

	stubborn := entries.await(t, "work under a lost lease will not stop")
	require.Equal(t, "lease", stubborn["component"])
	require.Equal(t, "sample", stubborn["lease"])
	require.NotNil(t, stubborn["err"])
	close(release)
	require.NoError(t, awaitReturned(t, returned))
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

// holderLog stands in for the logger a holder passes in — the cron job's, the
// leader's, the lock's — and records what the protocol writes through it. It
// implements the two methods the protocol calls and no more: an entry written
// through any other runs into the embedded nil interface, so a call site
// added later cannot go unnoticed.
type holderLog struct {
	types.Logger

	mu      sync.Mutex
	entries []holderEntry
}

// holderEntry is one recorded entry: its message and its fields, by key.
type holderEntry struct {
	msg    string
	fields map[string]any
}

// newHolderLog returns a logger recording what is written through it.
func newHolderLog() *holderLog {
	return &holderLog{}
}

func (l *holderLog) Warnw(msg string, keysAndValues ...any) {
	l.record(msg, keysAndValues)
}

func (l *holderLog) Errorw(msg string, keysAndValues ...any) {
	l.record(msg, keysAndValues)
}

// record keeps one entry, its alternating key/value fields turned into a map.
func (l *holderLog) record(msg string, keysAndValues []any) {
	fields := make(map[string]any, len(keysAndValues)/2)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[i+1]
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, holderEntry{msg: msg, fields: fields})
}

// await returns the fields of the first entry written with msg, waiting for
// it: the entries come from the renewal goroutine and the grace timer, not
// from the test's own. It fails the test when none comes in time.
func (l *holderLog) await(t *testing.T, msg string) map[string]any {
	t.Helper()

	deadline := time.After(5 * time.Second)
	for {
		l.mu.Lock()
		for _, entry := range l.entries {
			if entry.msg == msg {
				l.mu.Unlock()
				return entry.fields
			}
		}
		l.mu.Unlock()

		select {
		case <-time.After(5 * time.Millisecond):
		case <-deadline:
			t.Fatalf("no entry with msg %q was written through the holder's logger", msg)
			return nil
		}
	}
}

// TestHoldEndsAtTheDeadlineWhenARenewalFailsShortOfItsBound proves the
// deadline holds even when a renewal fails a moment before its bound: the
// next attempt is due at the deadline, not a whole interval later, so the
// holder does not sleep on past the margin its successor's claim respects.
func TestHoldEndsAtTheDeadlineWhenARenewalFailsShortOfItsBound(t *testing.T) {
	const interval, deadline = 200 * time.Millisecond, 400 * time.Millisecond
	t.Cleanup(SetTimings(600*time.Millisecond, interval, deadline, 100*time.Millisecond))
	// The failure lands at three quarters of the way to the deadline: after
	// the interval, before the bound, and a whole interval after it would
	// overshoot the deadline.
	withDelayedFailingDatabase(t, 150*time.Millisecond)

	begin := time.Now()
	held, stop := Hold(context.Background(), &Handle{name: "sample", holder: "sample", claimedAt: begin}, newHolderLog())
	defer stop()

	awaitDone(held, t)
	require.ErrorIs(t, context.Cause(held), ErrLost)
	elapsed := time.Since(begin)
	require.GreaterOrEqual(t, elapsed, deadline, "a failed renewal must not end the lease before the deadline")
	require.Less(t, elapsed, deadline+interval*2/5, "the lease must end at the deadline, not an interval later")
}

// delayedFailingPool is a connection pool whose every statement fails after
// delay: the database that answers slowly and badly.
type delayedFailingPool struct {
	delay time.Duration
}

var errPoolFailure = errors.New("sample pool failure")

func (p delayedFailingPool) ExecContext(ctx context.Context, _ string, _ ...any) (sql.Result, error) {
	select {
	case <-time.After(p.delay):
		return nil, errPoolFailure
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p delayedFailingPool) QueryContext(ctx context.Context, _ string, _ ...any) (*sql.Rows, error) {
	select {
	case <-time.After(p.delay):
		return nil, errPoolFailure
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (delayedFailingPool) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

func (delayedFailingPool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errPoolFailure
}

// withDelayedFailingDatabase points the engine at a handle whose statements
// fail after delay, and restores the suite's database afterwards. The
// handle keeps a real dialect, so the protocol gets as far as the statement.
func withDelayedFailingDatabase(t *testing.T, delay time.Duration) {
	t.Helper()

	slow, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	pool := delayedFailingPool{delay: delay}
	slow.ConnPool = pool
	slow.Statement.ConnPool = pool

	original := dbruntime.DB
	dbruntime.DB = slow
	t.Cleanup(func() { dbruntime.DB = original })
}
