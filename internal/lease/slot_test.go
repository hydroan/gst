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
		require.NoError(t, won[0].Finish(ctx, time.Time{}))
	})

	t.Run("name given back", func(t *testing.T) {
		name := uniqueName(t)
		h, claimed, err := ClaimSlot(ctx, name, first)
		require.NoError(t, err)
		require.True(t, claimed)
		require.NoError(t, h.Finish(ctx, time.Time{}))

		won := claimConcurrently(t, func() (*Handle, bool, error) { return ClaimSlot(ctx, name, first.Add(time.Minute)) })
		require.Len(t, won, 1, "exactly one claimant takes the next instant")
		require.EqualValues(t, 2, won[0].Term(), "the term moves once, by the winner")
		require.NoError(t, won[0].Finish(ctx, time.Time{}))
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
	require.NoError(t, h.Finish(ctx, time.Time{}))

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
		require.NoError(t, other.Finish(ctx, time.Time{}))
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
// back, nothing is left to run again, and the instants the round overran stay
// skipped — the last of them on record, as long as the database clock has
// reached it; once the name has changed hands it still settles the instant —
// a round that ran to its end, however late, never runs a second time —
// reporting ErrLost, but records no instant it overran and leaves alone a
// later instant claimed since.
func TestFinishRecordsTheRoundRanToItsEnd(t *testing.T) {
	ctx := context.Background()
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	t.Run("name still held", func(t *testing.T) {
		name := uniqueName(t)
		h, claimed, err := ClaimSlot(ctx, name, first)
		require.NoError(t, err)
		require.True(t, claimed)

		require.NoError(t, h.Finish(ctx, time.Time{}))
		require.Zero(t, readSlots(t, name).UnfinishedSlotMs)
		next, claimed, err := ClaimSlot(ctx, name, first.Add(time.Minute))
		require.NoError(t, err)
		require.True(t, claimed, "the lease was given back: the next instant is free at once")
		_, gaveUp := next.Superseded()
		require.False(t, gaveUp, "an instant run to its end is not given up by the next")
		require.NoError(t, next.Finish(ctx, time.Time{}))
	})

	t.Run("instants overrun", func(t *testing.T) {
		name := uniqueName(t)
		h, claimed, err := ClaimSlot(ctx, name, first)
		require.NoError(t, err)
		require.True(t, claimed)

		// The round ends two instants late.
		overrun := first.Add(2 * time.Minute)
		require.NoError(t, h.Finish(ctx, overrun))
		last, _, err := LastSlot(ctx, name)
		require.NoError(t, err)
		require.Equal(t, overrun, last, "the last instant the round overran is on record")
		for _, skipped := range []time.Time{first.Add(time.Minute), overrun} {
			_, claimed, err = ClaimSlot(ctx, name, skipped)
			require.NoError(t, err)
			require.False(t, claimed, "an instant the round overran stays skipped")
		}
		next, claimed, err := ClaimSlot(ctx, name, overrun.Add(time.Minute))
		require.NoError(t, err)
		require.True(t, claimed, "the instant after the overrun is free")
		_, gaveUp := next.Superseded()
		require.False(t, gaveUp)
		require.NoError(t, next.Finish(ctx, time.Time{}))
	})

	t.Run("instant yet to come by the database clock", func(t *testing.T) {
		name := uniqueName(t)
		h, claimed, err := ClaimSlot(ctx, name, first)
		require.NoError(t, err)
		require.True(t, claimed)

		// A clock running ahead of the database's names an instant still to come.
		require.NoError(t, h.Finish(ctx, time.Now().Add(time.Hour)))
		require.Equal(t, first.UnixMilli(), readSlots(t, name).SlotMs, "an instant the database clock has yet to reach stays free to claim")
	})

	t.Run("name claimed again elsewhere", func(t *testing.T) {
		name := uniqueName(t)
		late := cutShort(t, name, first)
		again := claimAgain(t, name)

		require.ErrorIs(t, late.Finish(ctx, first.Add(time.Minute)), ErrLost, "the name was no longer the late round's to give back")
		row := readSlots(t, name)
		require.Zero(t, row.UnfinishedSlotMs, "the late round ran to its end all the same")
		require.Equal(t, first.UnixMilli(), row.SlotMs, "a round that lost the name records no instant it overran")
		require.NoError(t, again.Finish(ctx, time.Time{}))
	})

	t.Run("later instant claimed since", func(t *testing.T) {
		name := uniqueName(t)
		late := cutShort(t, name, first)
		next, claimed, err := ClaimSlot(ctx, name, first.Add(time.Minute))
		require.NoError(t, err)
		require.True(t, claimed)

		require.ErrorIs(t, late.Finish(ctx, first.Add(2*time.Minute)), ErrLost)
		row := readSlots(t, name)
		require.Equal(t, first.Add(time.Minute).UnixMilli(), row.UnfinishedSlotMs,
			"a late round settles its own instant only, never the one claimed after it")
		require.Equal(t, first.Add(time.Minute).UnixMilli(), row.SlotMs, "nor does it move the record past the instant claimed after it")
		require.NoError(t, next.Finish(ctx, time.Time{}))
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

		require.NoError(t, h.Finish(ctx, first.Add(time.Minute)), "a holder known lost does not try to give the name back, and reports nothing")
		row := readSlots(t, name)
		require.Zero(t, row.UnfinishedSlotMs, "the round ran to its end all the same")
		require.Equal(t, first.UnixMilli(), row.SlotMs, "a holder known lost records no instant it overran")
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
	require.NoError(t, next.Finish(ctx, time.Time{}))
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

// TestUnfinishedSlotsCountsOnlyTheLastInstantClaimed proves a round is run a
// second time, or given up by the next claim, only while its unfinished mark
// names the last instant claimed under the name. A row holding the columns'
// defaults has nothing to run again and nothing to give up, and neither has
// a row whose last instant claimed has moved past its unfinished mark,
// whatever wrote it.
func TestUnfinishedSlotsCountsOnlyTheLastInstantClaimed(t *testing.T) {
	ctx := context.Background()
	first := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	t.Run("no instant on record as unfinished", func(t *testing.T) {
		name := uniqueName(t)
		now := dbruntime.NowUTC()

		// A claimed instant beside the columns' defaults, the lease given back.
		require.NoError(t, dbruntime.DB.Exec(
			"INSERT INTO "+table+" (name, holder, instance, term, expires_at_ms, slot_ms, created_at, updated_at) VALUES (?, ?, ?, 3, 0, ?, ?, ?)",
			name, "sample-holder", "sample-instance", first.UnixMilli(), now, now).Error)

		require.Empty(t, unfinished(t, name), "a row with no instant on record as unfinished runs nothing again")
		next, claimed, err := ClaimSlot(ctx, name, first.Add(time.Minute))
		require.NoError(t, err)
		require.True(t, claimed)
		_, gaveUp := next.Superseded()
		require.False(t, gaveUp, "nor does the next instant give anything up")
		require.NoError(t, next.Finish(ctx, time.Time{}))
	})

	t.Run("unfinished mark behind the last instant claimed", func(t *testing.T) {
		name := uniqueName(t)
		cutShort(t, name, first)
		// A later instant on record over the round cut short, the unfinished
		// mark left on the earlier instant and the lease given back.
		require.NoError(t, dbruntime.DB.Exec(
			"UPDATE "+table+" SET holder = ?, instance = ?, term = term + 1, expires_at_ms = 0, slot_ms = ? WHERE name = ?",
			"sample-holder", "sample-instance", first.Add(time.Minute).UnixMilli(), name).Error)

		require.Empty(t, unfinished(t, name), "an unfinished mark behind the last instant claimed is not run again")
		next, claimed, err := ClaimSlot(ctx, name, first.Add(2*time.Minute))
		require.NoError(t, err)
		require.True(t, claimed)
		_, gaveUp := next.Superseded()
		require.False(t, gaveUp, "nor given up by the next instant")
		require.NoError(t, next.Finish(ctx, time.Time{}))
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
