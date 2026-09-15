package lease

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// TestMain runs the suite against the dialect under test; the Makefile test
// target repeats the package once per dialect, because the protocol reads
// each database's clock through SQL of its own. The lease table registers
// itself when the package is linked, so the bootstrap creates it.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{Database: testutil.DatabaseUnderTest()})
}

// TestNowExpressionReadsTheDatabaseClock proves the clock expression of the
// dialect under test is one the database evaluates, and that it reads the
// present in UTC milliseconds — the one clock the protocol goes by.
func TestNowExpressionReadsTheDatabaseClock(t *testing.T) {
	db, now, err := primary()
	require.NoError(t, err)

	var got int64
	require.NoError(t, db.Raw("SELECT "+now).Scan(&got).Error)
	require.InDelta(t, time.Now().UnixMilli(), got, float64((5 * time.Minute).Milliseconds()),
		"the database clock must read the present in UTC milliseconds")
}

// TestNowExpressionRefusesAClickhousePrimary proves the protocol refuses the
// one primary database it cannot run on, instead of running wrong on it,
// and that Available answers the same for the database under test.
func TestNowExpressionRefusesAClickhousePrimary(t *testing.T) {
	_, err := nowExpression("clickhouse")
	require.ErrorIs(t, err, ErrUnsupportedDatabase)
	for _, dialect := range []string{"mysql", "postgres", "sqlite"} {
		now, err := nowExpression(dialect)
		require.NoError(t, err)
		require.NotEmpty(t, now)
	}
	require.NoError(t, Available(), "the database under test carries leases")
}

// TestClaimIsExclusiveAcrossConcurrentClaimants proves the contract: of many
// processes claiming one name at once, exactly one holds it.
func TestClaimIsExclusiveAcrossConcurrentClaimants(t *testing.T) {
	name := uniqueName(t)

	const claimants = 16
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners []*Handle
		errs    []error
	)
	for range claimants {
		wg.Go(func() {
			h, claimed, err := Claim(context.Background(), name)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
			}
			if claimed {
				winners = append(winners, h)
			}
		})
	}
	wg.Wait()

	require.Empty(t, errs)
	require.Len(t, winners, 1, "exactly one claimant may hold the name")
	require.NoError(t, winners[0].Release(context.Background()))
}

// TestClaimIsRefusedWhileHeldAndFreeAfterRelease proves a held name is
// refused to everyone else until the holder releases it, and that the next
// claim starts the next term under a token of its own.
func TestClaimIsRefusedWhileHeldAndFreeAfterRelease(t *testing.T) {
	ctx := context.Background()
	name := uniqueName(t)

	first, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)
	require.EqualValues(t, 1, first.Term())

	_, claimed, err = Claim(ctx, name)
	require.NoError(t, err)
	require.False(t, claimed, "a held name is refused")

	require.NoError(t, first.Release(ctx))
	second, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed, "a released name is free")
	require.EqualValues(t, 2, second.Term(), "every change of hands starts the next term")
	require.NotEqual(t, first.holder, second.holder, "a token is never reused")
	require.NoError(t, second.Release(ctx))
}

// TestExpiredLeaseIsClaimedByAnother proves a holder that stops renewing
// loses the name once the database clock passes the expiry: the next claim
// wins, and the old holder's renewal, release and verification all report
// the loss.
func TestExpiredLeaseIsClaimedByAnother(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	old, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)

	var successor *Handle
	require.Eventually(t, func() bool {
		h, claimed, claimErr := Claim(ctx, name)
		require.NoError(t, claimErr)
		successor = h
		return claimed
	}, 5*time.Second, 20*time.Millisecond, "the name must be free once the lease expired")
	require.Equal(t, old.Term()+1, successor.Term())

	require.ErrorIs(t, old.Renew(ctx), ErrLost)
	require.ErrorIs(t, old.Release(ctx), ErrLost)
	require.ErrorIs(t, Verify(WithHandle(ctx, old), dbruntime.DB), ErrLost)
	require.NoError(t, Verify(WithHandle(ctx, successor), dbruntime.DB))
	require.NoError(t, successor.Release(ctx))
}

// TestRenewKeepsTheLeaseBeyondItsDuration proves renewals move the expiry:
// a holder renewing on time keeps the name for as long as it likes, and
// every claim by another is refused meanwhile.
func TestRenewKeepsTheLeaseBeyondItsDuration(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)

	// Renew across several durations' worth of time, checking between
	// renewals that no one else gets in.
	deadline := time.Now().Add(3 * leaseDuration)
	for time.Now().Before(deadline) {
		require.NoError(t, holder.Renew(ctx))
		_, claimed, err := Claim(ctx, name)
		require.NoError(t, err)
		require.False(t, claimed, "a renewed lease is not free")
		time.Sleep(renewInterval)
	}
	require.NoError(t, holder.Release(ctx))
}

// TestClaimSlotRunsAnInstantOnce proves the scheduler's claim: an instant is
// claimed once under a name, an instant already claimed — or an earlier one
// — is refused even once the lease is released, and a later instant is free.
// LastSlot reports the instant last claimed, and nothing for a name never
// claimed.
func TestClaimSlotRunsAnInstantOnce(t *testing.T) {
	ctx := context.Background()
	name := uniqueName(t)
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)

	_, found, err := LastSlot(ctx, name)
	require.NoError(t, err)
	require.False(t, found, "a name never claimed has no last slot")

	h, claimed, err := ClaimSlot(ctx, name, first)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, first.UnixMilli(), h.slotMs)
	require.NoError(t, h.Release(ctx))

	slot, found, err := LastSlot(ctx, name)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, first, slot)

	_, claimed, err = ClaimSlot(ctx, name, first)
	require.NoError(t, err)
	require.False(t, claimed, "an instant already claimed is refused")
	_, claimed, err = ClaimSlot(ctx, name, first.Add(-time.Minute))
	require.NoError(t, err)
	require.False(t, claimed, "an instant before the last claimed one is refused")

	h, claimed, err = ClaimSlot(ctx, name, second)
	require.NoError(t, err)
	require.True(t, claimed, "the next instant is free")
	require.Equal(t, second.UnixMilli(), h.slotMs)
	require.EqualValues(t, 2, h.Term())
	require.NoError(t, h.Release(ctx))
}

// TestTransactionUnderALeaseVerifiesIt proves the transaction guard: a
// transaction opened on a context under a held lease runs, with the term at
// hand, and one opened under a lost lease returns ErrLost before a single
// statement of its own runs. A context under no lease is untouched.
func TestTransactionUnderALeaseVerifiesIt(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)

	ran := false
	require.NoError(t, database.Transaction(WithHandle(ctx, holder), func(txCtx context.Context) error {
		ran = true
		term, ok := TermFromContext(txCtx)
		require.True(t, ok, "the term must reach the work inside the transaction")
		require.Equal(t, holder.Term(), term)
		return nil
	}))
	require.True(t, ran)

	// Let the lease expire and pass to another holder.
	var successor *Handle
	require.Eventually(t, func() bool {
		h, claimed, claimErr := Claim(ctx, name)
		require.NoError(t, claimErr)
		successor = h
		return claimed
	}, 5*time.Second, 20*time.Millisecond)
	t.Cleanup(func() { _ = successor.Release(ctx) })

	ran = false
	err = database.Transaction(WithHandle(ctx, holder), func(context.Context) error {
		ran = true
		return nil
	})
	require.ErrorIs(t, err, ErrLost)
	require.False(t, ran, "no statement of a transaction under a lost lease may run")

	ran = false
	require.NoError(t, database.Transaction(ctx, func(context.Context) error {
		ran = true
		return nil
	}))
	require.True(t, ran, "a transaction under no lease is untouched")
}

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

// TestValidateNameRefusesWhatTheColumnCannotHold pins the name rule to the
// column it protects: a name as wide as the column passes, one byte more is
// refused, and the column is as wide as the rule says — so a table change
// and the rule cannot drift apart.
func TestValidateNameRefusesWhatTheColumnCannotHold(t *testing.T) {
	require.NoError(t, ValidateName(strings.Repeat("n", nameMaxLength)))
	require.ErrorContains(t, ValidateName(strings.Repeat("n", nameMaxLength+1)), "longer than")
	require.ErrorContains(t, ValidateName(""), "empty name")

	parsed, err := schema.Parse(&row{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	require.Equal(t, nameMaxLength, parsed.LookUpField("Name").Size, "the name column must be exactly as wide as ValidateName allows")
}

// TestClaimRefusesAnOverlongName proves the table is never asked to hold a
// name it would truncate or refuse: the claim fails before the statement, on
// every dialect alike.
func TestClaimRefusesAnOverlongName(t *testing.T) {
	_, claimed, err := Claim(context.Background(), strings.Repeat("n", nameMaxLength+1))
	require.ErrorContains(t, err, "longer than")
	require.False(t, claimed)
}

// TestRunReturnsWhatTheWorkReturned proves Run hands the work's outcome back
// as is, and turns a panic in the work into an error carrying its stack.
func TestRunReturnsWhatTheWorkReturned(t *testing.T) {
	ctx := context.Background()

	require.NoError(t, Run(ctx, "sample", func(context.Context) error { return nil }))
	require.ErrorContains(t, Run(ctx, "sample", func(context.Context) error { return errors.New("sample failure") }), "sample failure")
	err := Run(ctx, "sample", func(context.Context) error { panic("sample panic") })
	require.ErrorContains(t, err, "sample panic")
	require.Contains(t, fmt.Sprintf("%+v", err), "lease_test.go", "the error must carry the stack of the panic site")
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

// withFastProtocol shrinks the protocol's timings so expiry and renewal play
// out in milliseconds, and restores them afterwards.
func withFastProtocol(t *testing.T) {
	t.Helper()

	t.Cleanup(SetTimings(300*time.Millisecond, 50*time.Millisecond, 150*time.Millisecond, 100*time.Millisecond))
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

// uniqueName returns a coordinated name no other test uses: the table is
// shared by the suite, and leases outlive the test that claimed them.
func uniqueName(t *testing.T) string {
	t.Helper()

	token, err := newHolder()
	require.NoError(t, err)
	return fmt.Sprintf("sample:%s:%s", t.Name(), token[:8])
}
