package lease

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/instance"
)

// This file holds the scheduler's side of the protocol: an instant of a job
// claimed once across the deployment, the end of its round recorded along
// with the instants it overran, and a round cut short found and claimed a
// second time. The claims of any other name, and what every claim does once
// made, are in lease.go.

// Unfinished is an instant claimed under a scheduler's name whose round has
// not run to its end: the process running it shut down or died, or its lease
// was lost, before the round returned.
type Unfinished struct {
	// Name is the name the instant was claimed under.
	Name string
	// Slot is the instant.
	Slot time.Time
	// Rerun reports whether the instant had been claimed a second time
	// already, and that round was cut short too. UnfinishedSlots never
	// returns such an instant.
	Rerun bool
	// term is the term the row was read in: the instant is claimed again
	// only while the name has not changed hands since.
	term uint64
}

// slotState is what the claim of an instant reads before it writes.
type slotState struct {
	Term             uint64
	SlotMs           int64
	UnfinishedSlotMs int64
	RerunSlotMs      int64
	// Expired is 1 when the lease has expired or been released, by the
	// database clock.
	Expired int
}

// unfinishedRow is one row UnfinishedSlots finds.
type unfinishedRow struct {
	Name   string
	Term   uint64
	SlotMs int64
}

// ClaimSlot is Claim for an instant of a scheduler's job, slot, under name:
// the name must be free, and slot later than the last instant on record under
// it — the last instant claimed, or the last one a round that ran to its end
// recorded as having come while it held the name, see Finish. The claim
// records slot as the last instant claimed, and as unfinished until Finish;
// it is refused when someone holds the name or slot is not later than the
// instant on record. A cluster then claims each instant of a job once,
// whichever replica gets there first, and an instant already claimed, or
// skipped while a round held the name, is refused everywhere; only a round cut
// short is claimed a second time, see ClaimRerun.
//
// The instant claimed before under name, when its round has not run to its
// end — cut short, and not run a second time yet, or cut short the second
// time too — is given up by the claim and never runs again; the handle names
// it, see Handle.Superseded.
func ClaimSlot(ctx context.Context, name string, slot time.Time) (*Handle, bool, error) {
	if err := ValidateName(name); err != nil {
		return nil, false, err
	}
	db, now, err := primary()
	if err != nil {
		return nil, false, err
	}
	holder, err := newHolder()
	if err != nil {
		return nil, false, err
	}
	slotMs := slot.UnixMilli()
	updatedAt := dbruntime.NowUTC()
	ctx, cancel := bounded(ctx)
	defer cancel()

	var current slotState
	res := db.WithContext(ctx).Raw(
		fmt.Sprintf("SELECT term, slot_ms, unfinished_slot_ms, rerun_slot_ms, CASE WHEN expires_at_ms <= %s THEN 1 ELSE 0 END AS expired FROM %s WHERE name = ?", now, table),
		name).Scan(&current)
	if res.Error != nil {
		return nil, false, errors.Wrapf(res.Error, "claim lease %q", name)
	}
	if res.RowsAffected == 0 {
		// The table has never seen the name: the insert is the claim, and
		// inserting nothing means another process inserted first.
		return insertClaim(ctx, db, now, name, holder, slotMs, updatedAt, time.Now())
	}
	if current.Expired == 0 || current.SlotMs >= slotMs {
		return nil, false, nil
	}

	claimedAt := time.Now()
	res = db.WithContext(ctx).Exec(
		fmt.Sprintf("UPDATE %s SET holder = ?, instance = ?, term = term + 1, expires_at_ms = %s + ?, slot_ms = ?, unfinished_slot_ms = ?, updated_at = ? WHERE name = ? AND term = ? AND expires_at_ms <= %s", table, now, now),
		holder, instance.ID(), leaseDuration.Milliseconds(), slotMs, slotMs, updatedAt, name, current.Term)
	if res.Error != nil {
		return nil, false, errors.Wrapf(res.Error, "claim lease %q", name)
	}
	if res.RowsAffected == 0 {
		// The term moved between the read and the write: another replica
		// claimed first.
		return nil, false, nil
	}
	h := newHandle(name, holder, current.Term+1, claimedAt)
	h.slotMs = slotMs
	// Only the last instant claimed can be unfinished: a mark naming any
	// other instant is settled, and the claim gives nothing up for it.
	if current.UnfinishedSlotMs != 0 && current.UnfinishedSlotMs == current.SlotMs {
		h.superseded = &Unfinished{
			Name:  name,
			Slot:  time.UnixMilli(current.UnfinishedSlotMs).UTC(),
			Rerun: current.RerunSlotMs == current.UnfinishedSlotMs,
		}
	}
	return h, true, nil
}

// Superseded returns the instant the claim of h gave up, and whether it gave
// one up: the instant claimed before under the name, whose round had not run
// to its end — cut short, and not run a second time yet, or cut short the
// second time too — and that never runs again now. Only a handle of
// ClaimSlot gives one up.
func (h *Handle) Superseded() (Unfinished, bool) {
	if h.superseded == nil {
		return Unfinished{}, false
	}
	return *h.superseded, true
}

// Finish records that the round for the instant h was claimed for ran to its
// end, so the instant never runs again, and gives the name back, the way
// Release does. passed is the last instant of the round's schedule to have
// come by the time the round ended. When it is later than the round's own
// instant, the round overran the instants up to it: none of them was claimed —
// a claim of one would have refused the round's own, and the round's lease
// refused every claim while it held the name — and the record takes passed
// along as the last instant on record, so they stay skipped, every later
// claim of them refused, a start-up catch-up included. An instant the database
// clock has yet to reach is not taken along, so a caller whose clock runs
// ahead does not skip an instant the other replicas are still to claim.
//
// When the name is no longer this holder's the end is recorded all the same,
// unless a later instant has been claimed since, and passed is not taken
// along: other replicas may have claimed the name meanwhile. ErrLost then
// reports the name was not given back — unless h is known lost, see Lost, and
// giving it back was not tried. Only a handle of ClaimSlot or ClaimRerun has
// an instant to finish.
func (h *Handle) Finish(ctx context.Context, passed time.Time) error {
	if h.slotMs == 0 {
		return errors.Newf("finish lease %q: the claim was for no instant", h.name)
	}
	db, now, err := primary()
	if err != nil {
		return err
	}
	updatedAt := dbruntime.NowUTC()
	if !h.Lost() {
		passedMs := passed.UnixMilli()
		res := db.WithContext(ctx).Exec(
			fmt.Sprintf("UPDATE %s SET expires_at_ms = 0, unfinished_slot_ms = 0, slot_ms = CASE WHEN slot_ms < ? AND ? <= %s THEN ? ELSE slot_ms END, updated_at = ? WHERE name = ? AND holder = ?", table, now),
			passedMs, passedMs, passedMs, updatedAt, h.name, h.holder)
		if res.Error != nil {
			return errors.Wrapf(res.Error, "finish lease %q", h.name)
		}
		if res.RowsAffected == 1 {
			return nil
		}
	}

	// The name changed hands, or expired with no one to take it yet: the
	// round did run to its end, and the record says so, as long as the
	// instant is still the one on record as unfinished.
	res := db.WithContext(ctx).Exec(
		fmt.Sprintf("UPDATE %s SET unfinished_slot_ms = 0, updated_at = ? WHERE name = ? AND unfinished_slot_ms = ?", table),
		updatedAt, h.name, h.slotMs)
	if res.Error != nil {
		return errors.Wrapf(res.Error, "finish lease %q", h.name)
	}
	if h.Lost() {
		return nil
	}
	return errors.Wrapf(ErrLost, "finish lease %q", h.name)
}

// UnfinishedSlots returns the instants, under the names given, whose round
// was cut short and that are free to claim a second time: the round has not
// run to its end, the instant has not been claimed a second time already, no
// later instant has been claimed since, and the lease has expired or been
// released. One statement answers for all the names.
func UnfinishedSlots(ctx context.Context, names []string) ([]Unfinished, error) {
	if len(names) == 0 {
		return nil, nil
	}
	db, now, err := primary()
	if err != nil {
		return nil, err
	}
	ctx, cancel := bounded(ctx)
	defer cancel()

	var rows []unfinishedRow
	res := db.WithContext(ctx).Raw(
		fmt.Sprintf("SELECT name, term, slot_ms FROM %s WHERE name IN ? AND unfinished_slot_ms = slot_ms AND rerun_slot_ms < slot_ms AND expires_at_ms <= %s", table, now),
		names).Scan(&rows)
	if res.Error != nil {
		return nil, errors.Wrap(res.Error, "find the unfinished instants of leases")
	}
	found := make([]Unfinished, 0, len(rows))
	for _, r := range rows {
		found = append(found, Unfinished{Name: r.Name, Slot: time.UnixMilli(r.SlotMs).UTC(), term: r.Term})
	}
	return found, nil
}

// ClaimRerun claims u's instant a second time, u as UnfinishedSlots found it,
// and returns the handle and true when the instant is still free to claim a
// second time: the name has not changed hands since it was found. It returns
// false when the instant was claimed again elsewhere, its first round was
// recorded finished after all, a later instant was claimed, or the name is
// held. The round runs under the handle like any other and ends with Finish,
// or with Release when it is cut short again, which leaves the instant
// unfinished for good.
func ClaimRerun(ctx context.Context, u Unfinished) (*Handle, bool, error) {
	if err := ValidateName(u.Name); err != nil {
		return nil, false, err
	}
	db, now, err := primary()
	if err != nil {
		return nil, false, err
	}
	holder, err := newHolder()
	if err != nil {
		return nil, false, err
	}
	updatedAt := dbruntime.NowUTC()
	ctx, cancel := bounded(ctx)
	defer cancel()

	claimedAt := time.Now()
	res := db.WithContext(ctx).Exec(
		fmt.Sprintf("UPDATE %s SET holder = ?, instance = ?, term = term + 1, expires_at_ms = %s + ?, rerun_slot_ms = slot_ms, updated_at = ? WHERE name = ? AND term = ? AND unfinished_slot_ms = slot_ms AND rerun_slot_ms < slot_ms AND expires_at_ms <= %s", table, now, now),
		holder, instance.ID(), leaseDuration.Milliseconds(), updatedAt, u.Name, u.term)
	if res.Error != nil {
		return nil, false, errors.Wrapf(res.Error, "claim lease %q again", u.Name)
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	h := newHandle(u.Name, holder, u.term+1, claimedAt)
	h.slotMs = u.Slot.UnixMilli()
	return h, true, nil
}

// LastSlot returns the last instant on record under name — the last claimed,
// or the last a round that ran to its end took along, see Finish — and whether
// the name has ever been claimed. The scheduler reads it as it starts, to tell
// an instant no replica claimed or skipped from a job that has never run at
// all.
func LastSlot(ctx context.Context, name string) (time.Time, bool, error) {
	db, _, err := primary()
	if err != nil {
		return time.Time{}, false, err
	}
	ctx, cancel := bounded(ctx)
	defer cancel()

	var slotMs int64
	res := db.WithContext(ctx).Raw(fmt.Sprintf("SELECT slot_ms FROM %s WHERE name = ?", table), name).Scan(&slotMs)
	if res.Error != nil {
		return time.Time{}, false, errors.Wrapf(res.Error, "read the last slot of lease %q", name)
	}
	if res.RowsAffected == 0 {
		return time.Time{}, false, nil
	}
	return time.UnixMilli(slotMs).UTC(), true, nil
}
