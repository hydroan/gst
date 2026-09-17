package lease

import (
	"context"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestClaimSlotClaimsAnInstantOnce proves the scheduler's claim: an instant is
// claimed once under a name, an instant already claimed — or an earlier one
// — is refused even once the lease is released, and a later instant is free.
// LastSlot reports the instant last claimed, and nothing for a name never
// claimed.
func TestClaimSlotClaimsAnInstantOnce(t *testing.T) {
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
	require.EqualValues(t, 2, h.Term())
	require.NoError(t, h.Release(ctx))
	slot, found, err = LastSlot(ctx, name)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, second, slot, "the claim records the instant as the last one claimed")
}

// TestClaimSlotIsExclusiveAcrossConcurrentClaimants proves the claim of an
// instant, a read and a write, is as exclusive as a single statement: of
// many replicas claiming one instant at once, exactly one wins, both for a
// name the table has never seen — the insert decides — and for one whose
// lease has been given back — the conditions of the write decide. The term
// among them is pinned by TestClaimSlotRefusesARowChangedSinceItWasRead.
func TestClaimSlotIsExclusiveAcrossConcurrentClaimants(t *testing.T) {
	ctx := context.Background()
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	t.Run("name never claimed", func(t *testing.T) {
		name := uniqueName(t)

		won := claimConcurrently(t, func() (*Handle, bool, error) { return ClaimSlot(ctx, name, first) })
		require.Len(t, won, 1, "exactly one claimant takes the instant")
		require.EqualValues(t, 1, won[0].Term())
		require.NoError(t, won[0].Finish(ctx))
	})

	t.Run("name given back", func(t *testing.T) {
		name := uniqueName(t)
		h, claimed, err := ClaimSlot(ctx, name, first)
		require.NoError(t, err)
		require.True(t, claimed)
		require.NoError(t, h.Finish(ctx))

		won := claimConcurrently(t, func() (*Handle, bool, error) { return ClaimSlot(ctx, name, first.Add(time.Minute)) })
		require.Len(t, won, 1, "exactly one claimant takes the next instant")
		require.EqualValues(t, 2, won[0].Term(), "the term moves once, by the winner")
		require.NoError(t, won[0].Finish(ctx))
	})
}

// TestClaimSlotRefusesARowChangedSinceItWasRead proves the write of an
// instant's claim holds to the row its read found: another replica claims the
// same instant between the read and the write and runs its round to its end,
// so the lease is free again by the time the write comes, yet the write finds
// the term moved and the claim is refused — the instant runs once.
func TestClaimSlotRefusesARowChangedSinceItWasRead(t *testing.T) {
	ctx := context.Background()
	name := uniqueName(t)
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)

	h, claimed, err := ClaimSlot(ctx, name, first)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, h.Finish(ctx))

	// The other replica's claim and round run in the gap, right before the
	// write of the claim that read the row first; the claims it makes itself
	// pass through untouched.
	var (
		raced atomic.Bool
		other *Handle
		won   bool
	)
	const interleave = "test:claim_between_read_and_write"
	require.NoError(t, dbruntime.DB.Callback().Raw().Before("gorm:raw").Register(interleave, func(tx *gorm.DB) {
		if !strings.HasPrefix(tx.Statement.SQL.String(), "UPDATE "+table+" SET holder") ||
			!slices.Contains(tx.Statement.Vars, any(name)) || !raced.CompareAndSwap(false, true) {
			return
		}
		var claimErr error
		other, won, claimErr = ClaimSlot(ctx, name, second)
		require.NoError(t, claimErr)
		require.NoError(t, other.Finish(ctx))
	}))
	t.Cleanup(func() { _ = dbruntime.DB.Callback().Raw().Remove(interleave) })

	_, claimed, err = ClaimSlot(ctx, name, second)
	require.NoError(t, err)
	require.True(t, won, "the other replica claims the instant in the gap")
	require.False(t, claimed, "a claim whose row changed since its read is refused")
	require.EqualValues(t, 2, readSlots(t, name).Term, "the instant is claimed once")
}

// TestFinishRecordsTheRoundRanToItsEnd proves Finish settles the instant its
// handle was claimed for: while the name is the holder's it gives the lease
// back and nothing is left to run again; once the name has changed hands it
// still settles the instant — a round that ran to its end, however late,
// never runs a second time — reporting ErrLost, but leaves alone a later
// instant claimed since.
func TestFinishRecordsTheRoundRanToItsEnd(t *testing.T) {
	ctx := context.Background()
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	t.Run("name still held", func(t *testing.T) {
		name := uniqueName(t)
		h, claimed, err := ClaimSlot(ctx, name, first)
		require.NoError(t, err)
		require.True(t, claimed)

		require.NoError(t, h.Finish(ctx))
		require.Zero(t, readSlots(t, name).UnfinishedSlotMs)
		next, claimed, err := ClaimSlot(ctx, name, first.Add(time.Minute))
		require.NoError(t, err)
		require.True(t, claimed, "the lease was given back: the next instant is free at once")
		_, gaveUp := next.Superseded()
		require.False(t, gaveUp, "an instant run to its end is not given up by the next")
		require.NoError(t, next.Finish(ctx))
	})

	t.Run("name claimed again elsewhere", func(t *testing.T) {
		name := uniqueName(t)
		late := cutShort(t, name, first)
		again := claimAgain(t, name)

		require.ErrorIs(t, late.Finish(ctx), ErrLost, "the name was no longer the late round's to give back")
		require.Zero(t, readSlots(t, name).UnfinishedSlotMs, "the late round ran to its end all the same")
		require.NoError(t, again.Finish(ctx))
	})

	t.Run("later instant claimed since", func(t *testing.T) {
		name := uniqueName(t)
		late := cutShort(t, name, first)
		next, claimed, err := ClaimSlot(ctx, name, first.Add(time.Minute))
		require.NoError(t, err)
		require.True(t, claimed)

		require.ErrorIs(t, late.Finish(ctx), ErrLost)
		require.Equal(t, first.Add(time.Minute).UnixMilli(), readSlots(t, name).UnfinishedSlotMs,
			"a late round settles its own instant only, never the one claimed after it")
		require.NoError(t, next.Finish(ctx))
	})

	t.Run("lease known lost", func(t *testing.T) {
		withFastProtocol(t)
		name := uniqueName(t)
		h, claimed, err := ClaimSlot(ctx, name, first)
		require.NoError(t, err)
		require.True(t, claimed)
		held, stop := Hold(ctx, h, newHolderLog())
		defer stop()

		// Another replica takes the name behind the holder's back, and the
		// renewals find it gone.
		require.NoError(t, dbruntime.DB.Exec("UPDATE "+table+" SET holder = ?, term = term + 1 WHERE name = ?", "other-holder", name).Error)
		awaitDone(held, t)
		require.True(t, h.Lost())

		require.NoError(t, h.Finish(ctx), "a holder known lost does not try to give the name back, and reports nothing")
		require.Zero(t, readSlots(t, name).UnfinishedSlotMs, "the round ran to its end all the same")
	})
}

// TestRoundCutShortIsClaimedASecondTimeOnce proves the second claim: a round
// cut short and given back is found at once, one of the replicas racing for
// it wins, the instant is not found while that round holds it, and cut short
// again it is given up — found no more, and named as run a second time by
// the claim of the next instant.
func TestRoundCutShortIsClaimedASecondTimeOnce(t *testing.T) {
	ctx := context.Background()
	name := uniqueName(t)
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	cutShort(t, name, first)
	found, err := UnfinishedSlots(ctx, []string{name})
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, first, found[0].Slot)

	won := claimConcurrently(t, func() (*Handle, bool, error) { return ClaimRerun(ctx, found[0]) })
	require.Len(t, won, 1, "exactly one replica runs the round a second time")
	require.EqualValues(t, 2, won[0].Term())
	require.Empty(t, unfinished(t, name), "a round running a second time holds its instant")

	require.NoError(t, won[0].Release(ctx))
	require.Empty(t, unfinished(t, name), "an instant cut short a second time is never claimed a third")
	next, claimed, err := ClaimSlot(ctx, name, first.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, claimed)
	given, gaveUp := next.Superseded()
	require.True(t, gaveUp)
	require.Equal(t, Unfinished{Name: name, Slot: first, Rerun: true}, given)
	require.NoError(t, next.Finish(ctx))
}

// TestRoundOfACrashedHolderIsFoundOnceItsLeaseExpires proves a round whose
// process died, and so never gave its lease back, is found once the lease
// has expired and not before: until then the holder may still be running
// it.
func TestRoundOfACrashedHolderIsFoundOnceItsLeaseExpires(t *testing.T) {
	withFastProtocol(t)
	ctx := context.Background()
	name := uniqueName(t)
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	_, claimed, err := ClaimSlot(ctx, name, first)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Empty(t, unfinished(t, name), "a held lease may still be running its round")

	require.Eventually(t, func() bool {
		found, findErr := UnfinishedSlots(ctx, []string{name})
		return findErr == nil && len(found) == 1 && found[0].Slot.Equal(first)
	}, 5*time.Second, 20*time.Millisecond, "the round is found once the lease has expired")
}

// TestClaimSlotGivesUpARoundCutShort proves the claim of the next instant
// gives up the instant before it whose round was cut short and not yet run a
// second time: the handle names that instant, and a second claim of it made
// from what was found before is refused — while the next instant's round
// holds the name, and still once that round is cut short too, when the row
// is free to claim a second time again and only the term found tells the
// instant given up from the one to run.
func TestClaimSlotGivesUpARoundCutShort(t *testing.T) {
	ctx := context.Background()
	name := uniqueName(t)
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	cutShort(t, name, first)
	found, err := UnfinishedSlots(ctx, []string{name})
	require.NoError(t, err)
	require.Len(t, found, 1)

	next, claimed, err := ClaimSlot(ctx, name, first.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, claimed)
	given, gaveUp := next.Superseded()
	require.True(t, gaveUp)
	require.Equal(t, Unfinished{Name: name, Slot: first}, given)

	_, claimed, err = ClaimRerun(ctx, found[0])
	require.NoError(t, err)
	require.False(t, claimed, "an instant given up is not claimed a second time")

	require.NoError(t, next.Release(ctx))
	_, claimed, err = ClaimRerun(ctx, found[0])
	require.NoError(t, err)
	require.False(t, claimed, "nor once a later instant cut short is free to be")
	left := unfinished(t, name)
	require.Len(t, left, 1)
	require.Equal(t, first.Add(time.Minute), left[0].Slot, "the later instant is the one left to run a second time")
}

// TestUnfinishedSlotsLeavesOutWhatAnEarlierReleaseWrote proves the protocol
// before the unfinished and rerun columns runs nothing again and has nothing
// given up. A row it wrote before the columns existed — every instant it
// claimed counted as run then — holds 0 in both once they are added, and
// reads as settled. A row it claims a later instant on once they exist — an
// earlier release running beside this one in a rolling deployment, or
// rolled back to — keeps an unfinished instant its claim wrote nothing over:
// that instant is no longer the last one claimed, so it is settled too, and
// the instant the earlier release ran is not taken for it.
func TestUnfinishedSlotsLeavesOutWhatAnEarlierReleaseWrote(t *testing.T) {
	ctx := context.Background()
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	t.Run("row written before its columns", func(t *testing.T) {
		name := uniqueName(t)
		now := dbruntime.NowUTC()

		// The insert of the earlier protocol, whose lease was given back since.
		require.NoError(t, dbruntime.DB.Exec(
			"INSERT INTO "+table+" (name, holder, instance, term, expires_at_ms, slot_ms, created_at, updated_at) VALUES (?, ?, ?, 3, 0, ?, ?, ?)",
			name, "earlier-holder", "earlier-instance", first.UnixMilli(), now, now).Error)

		require.Empty(t, unfinished(t, name), "an instant claimed before the upgrade is not run again")
		next, claimed, err := ClaimSlot(ctx, name, first.Add(time.Minute))
		require.NoError(t, err)
		require.True(t, claimed)
		_, gaveUp := next.Superseded()
		require.False(t, gaveUp, "nor is it given up by the next instant")
		require.NoError(t, next.Finish(ctx))
	})

	t.Run("later instant claimed by an earlier release", func(t *testing.T) {
		name := uniqueName(t)
		cutShort(t, name, first)
		// The claim of the earlier protocol, given back since: it writes the
		// slot alone and leaves the unfinished column as it found it.
		require.NoError(t, dbruntime.DB.Exec(
			"UPDATE "+table+" SET holder = ?, instance = ?, term = term + 1, expires_at_ms = 0, slot_ms = ? WHERE name = ?",
			"earlier-holder", "earlier-instance", first.Add(time.Minute).UnixMilli(), name).Error)

		require.Empty(t, unfinished(t, name), "neither the instant the earlier release ran nor the one it moved past is run again")
		next, claimed, err := ClaimSlot(ctx, name, first.Add(2*time.Minute))
		require.NoError(t, err)
		require.True(t, claimed)
		_, gaveUp := next.Superseded()
		require.False(t, gaveUp, "the instant the earlier release moved past is not given up once more")
		require.NoError(t, next.Finish(ctx))
	})
}

// cutShort claims the instant at under name and gives the lease back without
// finishing the round, the way a shutdown cuts a round short, and returns
// the handle.
func cutShort(t *testing.T, name string, at time.Time) *Handle {
	t.Helper()

	h, claimed, err := ClaimSlot(context.Background(), name, at)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, h.Release(context.Background()))
	return h
}

// claimAgain finds the one round of name cut short and claims it a second
// time, and returns the handle.
func claimAgain(t *testing.T, name string) *Handle {
	t.Helper()

	found, err := UnfinishedSlots(context.Background(), []string{name})
	require.NoError(t, err)
	require.Len(t, found, 1)
	h, claimed, err := ClaimRerun(context.Background(), found[0])
	require.NoError(t, err)
	require.True(t, claimed)
	return h
}

// unfinished returns the instants of name UnfinishedSlots finds.
func unfinished(t *testing.T, name string) []Unfinished {
	t.Helper()

	found, err := UnfinishedSlots(context.Background(), []string{name})
	require.NoError(t, err)
	return found
}

// readSlots reads the scheduler's columns of name's row.
func readSlots(t *testing.T, name string) slotState {
	t.Helper()

	var state slotState
	require.NoError(t, dbruntime.DB.Raw("SELECT term, slot_ms, unfinished_slot_ms, rerun_slot_ms FROM "+table+" WHERE name = ?", name).Scan(&state).Error)
	return state
}

// claimConcurrently runs claim from 16 claimants at once and returns the
// handles of those that won, failing the test on any error.
func claimConcurrently(t *testing.T, claim func() (*Handle, bool, error)) []*Handle {
	t.Helper()

	const claimants = 16
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners []*Handle
		errs    []error
	)
	for range claimants {
		wg.Go(func() {
			h, claimed, err := claim()
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
	return winners
}
