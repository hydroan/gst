package lease

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/stretchr/testify/require"
)

// envTestDatabase overrides the dialect this suite runs against, the way the
// database suite's TestMain reads it: the Makefile test target repeats the
// package once per dialect, because the protocol reads each database's
// clock through SQL of its own.
const envTestDatabase = "GST_TEST_DATABASE"

// TestMain runs the suite against MySQL by default and against the dialect
// envTestDatabase names when it is set. The lease table registers itself
// when the package is linked, so the bootstrap creates it.
func TestMain(m *testing.M) {
	dbType := config.DBMySQL
	if override := os.Getenv(envTestDatabase); len(override) > 0 {
		dbType = config.DBType(override)
	}
	testutil.Run(m, testutil.Server{Database: dbType})
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

// TestHoldEndsWhenTheLeaseIsTakenAway proves the context Hold hands out
// ends with ErrLost once a renewal finds the lease gone. On SQLite, where
// Hold does not renew, it proves the context stays with its parent instead.
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

	if dialectOf(dbruntime.DB) == "sqlite" {
		select {
		case <-held.Done():
			t.Fatalf("on sqlite the held context must end with its parent only: %v", context.Cause(held))
		case <-time.After(3 * localDeadline):
		}
		return
	}
	select {
	case <-held.Done():
		require.ErrorIs(t, context.Cause(held), ErrLost)
	case <-time.After(5 * time.Second):
		t.Fatal("the held context must end once the lease is gone")
	}
}

// withFastProtocol shrinks the protocol's timings so expiry and renewal play
// out in milliseconds, and restores them afterwards.
func withFastProtocol(t *testing.T) {
	t.Helper()

	originalDuration, originalInterval, originalDeadline := leaseDuration, renewInterval, localDeadline
	leaseDuration = 300 * time.Millisecond
	renewInterval = 50 * time.Millisecond
	localDeadline = 150 * time.Millisecond
	t.Cleanup(func() {
		leaseDuration, renewInterval, localDeadline = originalDuration, originalInterval, originalDeadline
	})
}

// uniqueName returns a coordinated name no other test uses: the table is
// shared by the suite, and leases outlive the test that claimed them.
func uniqueName(t *testing.T) string {
	t.Helper()

	token, err := newHolder()
	require.NoError(t, err)
	return fmt.Sprintf("sample:%s:%s", t.Name(), token[:8])
}
