package lease

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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
		if claimErr != nil {
			// A failing assertion belongs on the test goroutine, not
			// inside the condition.
			return false
		}
		successor = h
		return claimed
	}, 5*time.Second, 20*time.Millisecond, "the name must be free once the lease expired")
	require.Equal(t, old.Term()+1, successor.Term())

	require.ErrorIs(t, old.Renew(ctx), ErrLost)
	require.ErrorIs(t, old.Release(ctx), ErrLost)
	require.ErrorIs(t, Verify(WithHandle(ctx, old), dbruntime.DB, dbruntime.DB), ErrLost)
	require.NoError(t, Verify(WithHandle(ctx, successor), dbruntime.DB, dbruntime.DB))
	require.NoError(t, successor.Release(ctx))
}

// TestClaimOfAnExpiredNameIsExclusiveAcrossConcurrentClaimants proves the
// exclusivity of the path a running deployment takes: the row exists and
// its lease has expired, and of many claimants racing for it through the
// UPDATE exactly one wins, with the term advanced once.
func TestClaimOfAnExpiredNameIsExclusiveAcrossConcurrentClaimants(t *testing.T) {
	ctx := context.Background()
	name := uniqueName(t)

	old, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, dbruntime.DB.Exec("UPDATE "+table+" SET expires_at_ms = 1 WHERE name = ?", name).Error)

	const claimants = 16
	winners := make(chan *Handle, claimants)
	claimErrs := make(chan error, claimants)
	var wg sync.WaitGroup
	for range claimants {
		wg.Go(func() {
			h, claimed, claimErr := Claim(ctx, name)
			if claimErr != nil {
				claimErrs <- claimErr
				return
			}
			if claimed {
				winners <- h
			}
		})
	}
	wg.Wait()
	close(winners)
	close(claimErrs)
	for claimErr := range claimErrs {
		require.NoError(t, claimErr)
	}

	var won []*Handle
	for h := range winners {
		won = append(won, h)
	}
	require.Len(t, won, 1, "exactly one claimant takes an expired name")
	require.Equal(t, old.Term()+1, won[0].Term())
	require.NoError(t, won[0].Release(ctx))
}

// TestVerifyRefusesAReleasedOrExpiredLease proves the guard reads the
// expiry, not just the holder: a lease its holder released, and one that
// expired with no one to take it yet, are both refused, although the row
// still names the holder.
func TestVerifyRefusesAReleasedOrExpiredLease(t *testing.T) {
	ctx := context.Background()
	name := uniqueName(t)

	released, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, Verify(WithHandle(ctx, released), dbruntime.DB, dbruntime.DB))
	require.NoError(t, released.Release(ctx))
	require.ErrorIs(t, Verify(WithHandle(ctx, released), dbruntime.DB, dbruntime.DB), ErrLost)

	expired, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, dbruntime.DB.Exec("UPDATE "+table+" SET expires_at_ms = 1 WHERE name = ?", name).Error)
	require.ErrorIs(t, Verify(WithHandle(ctx, expired), dbruntime.DB, dbruntime.DB), ErrLost)
}

// TestTransactionOnAnotherInstanceVerifiesAgainstThePrimary proves a
// transaction under a lease opened on another database instance — which
// carries no lease table — is still checked, against the primary: it runs
// while the lease is held and is refused once the name changed hands.
func TestTransactionOnAnotherInstanceVerifiesAgainstThePrimary(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	other, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	otherPool, err := other.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = otherPool.Close() })

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)

	ran := false
	require.NoError(t, database.TransactionOn(WithHandle(ctx, holder), other, func(context.Context) error {
		ran = true
		return nil
	}))
	require.True(t, ran, "a transaction on another instance runs while the lease is held")

	var successor *Handle
	require.Eventually(t, func() bool {
		h, claimed, claimErr := Claim(ctx, name)
		if claimErr != nil {
			// A failing assertion belongs on the test goroutine, not
			// inside the condition.
			return false
		}
		successor = h
		return claimed
	}, 5*time.Second, 20*time.Millisecond)
	t.Cleanup(func() { _ = successor.Release(ctx) })

	ran = false
	err = database.TransactionOn(WithHandle(ctx, holder), other, func(context.Context) error {
		ran = true
		return nil
	})
	require.ErrorIs(t, err, ErrLost)
	require.False(t, ran, "no statement of a transaction under a lost lease may run, on any instance")
}

// TestRenewKeepsTheLeaseBeyondItsDuration proves renewals move the expiry:
// a holder renewing on time keeps the name for as long as it likes, and
// every claim by another is refused meanwhile.
func TestRenewKeepsTheLeaseBeyondItsDuration(t *testing.T) {
	withTolerantProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)

	holder, claimed, err := Claim(ctx, name)
	require.NoError(t, err)
	require.True(t, claimed)

	// Renew across a couple of durations' worth of time, checking between
	// renewals that no one else gets in.
	deadline := time.Now().Add(2 * leaseDuration)
	for time.Now().Before(deadline) {
		require.NoError(t, holder.Renew(ctx))
		_, claimed, err := Claim(ctx, name)
		require.NoError(t, err)
		require.False(t, claimed, "a renewed lease is not free")
		time.Sleep(renewInterval)
	}
	require.NoError(t, holder.Release(ctx))
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
		if claimErr != nil {
			// A failing assertion belongs on the test goroutine, not
			// inside the condition.
			return false
		}
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

// TestClaimsAreBoundedByHalfTheLocalDeadline pins the bound the protocol's
// own statements carry when their caller has none: a database that stopped
// answering — a connection gone but not closed answers nothing and reports
// nothing — would otherwise hold a scheduler loop, a campaign or the sweep
// for as long as it stays silent.
func TestClaimsAreBoundedByHalfTheLocalDeadline(t *testing.T) {
	withFastProtocol(t)

	ctx, cancel := bounded(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	require.True(t, ok, "a statement of the protocol runs under a deadline of its own")
	// The fast protocol's half-deadline is well under the floor, so the floor
	// is what the statement gets here: the bound is for a connection that
	// answers nothing, not for a database that is merely busy.
	require.InDelta(t, statementFloor, time.Until(deadline), float64(20*time.Millisecond))

	t.Run("HalfTheDeadlineOnceItIsPastTheFloor", func(t *testing.T) {
		t.Cleanup(SetTimings(30*time.Second, 2*time.Second, 10*time.Second, 5*time.Second))
		ctx, cancel := bounded(context.Background())
		defer cancel()
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, localDeadline/2, time.Until(deadline), float64(20*time.Millisecond),
			"a statement slower than half the local deadline would not have kept a lease alive either")
	})
}

// TestBoundKeepsTheCallersDeadlineWhenItIsSooner proves the bound never
// stretches a caller's own: a process shutting down, or a caller that gave
// the claim a shorter window, still ends the statement when it said so.
func TestBoundKeepsTheCallersDeadlineWhenItIsSooner(t *testing.T) {
	withFastProtocol(t)

	caller, cancelCaller := context.WithTimeout(context.Background(), statementFloor/10)
	defer cancelCaller()
	ctx, cancel := bounded(caller)
	defer cancel()

	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.Less(t, time.Until(deadline), statementFloor)
}

// withFastProtocol shrinks the protocol's timings so expiry and renewal play
// out in milliseconds, and restores them afterwards.
func withFastProtocol(t *testing.T) {
	t.Helper()

	t.Cleanup(SetTimings(300*time.Millisecond, 50*time.Millisecond, 150*time.Millisecond, 100*time.Millisecond))
}

// withTolerantProtocol shortens the timings less than withFastProtocol, for
// tests proving that a holder renewing on time keeps its name. What those
// tests ride on is the margin: a holder gives the lease up the moment no
// renewal has succeeded within the deadline, so a machine that keeps the
// renewing goroutine — or its statement — waiting that long fails them for
// reasons of its own. Two seconds is well past what the suite running beside
// them under the race detector takes, half a second was not, and the lease
// outlives the deadline, so they prove the holder's diligence rather than the
// machine's speed.
func withTolerantProtocol(t *testing.T) {
	t.Helper()

	t.Cleanup(SetTimings(3*time.Second, 100*time.Millisecond, 2*time.Second, 300*time.Millisecond))
}

// withProductionProportions plays the protocol out at a fifth of the timings
// a deployment runs, keeping their proportions: a test proving the holder
// rides out what a deployment meets — a renewal that fails, one that answers
// slowly, the work's own transactions holding the one connection — has to
// face the retries and the deadline in the ratios a deployment runs, or
// ratios chosen for the test would prove it instead.
func withProductionProportions(t *testing.T) {
	t.Helper()

	t.Cleanup(SetTimings(leaseDuration/5, renewInterval/5, localDeadline/5, stepDownGrace/5))
}

// uniqueName returns a coordinated name no other test uses: the table is
// shared by the suite, and leases outlive the test that claimed them.
func uniqueName(t *testing.T) string {
	t.Helper()

	token, err := newHolder()
	require.NoError(t, err)
	return fmt.Sprintf("sample:%s:%s", t.Name(), token[:8])
}
