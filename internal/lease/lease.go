// Package lease is the coordination primitive the framework's distributed
// capabilities stand on: a name that at most one healthy process among those
// sharing the primary database holds at a time. The scheduler claims a name
// per job and instant, so a cluster runs each instant once; a capability
// that keeps a name for as long as its holder stays healthy, or claims one
// for the duration of a call, is built the same way.
//
// A lease is one row of gst_leases, updated in place: who holds the name
// (holder, a token minted per claim and never reused), which process that is
// (instance, for reading logs), the term (incremented every time the name
// changes hands, the fencing token for the world outside the database), when
// the lease expires (by the database clock, never a process clock) and, for
// the scheduler, the last instant claimed. Four single-row statements do all
// the work; no lock outlives its own statement:
//
//	claim    UPDATE gst_leases SET holder = :holder, instance = :instance,
//	             term = term + 1, expires_at_ms = :now + 15000 [, slot_ms = :slot]
//	          WHERE name = :name AND expires_at_ms <= :now [AND slot_ms < :slot]
//	         1 row: claimed. 0 rows: held by someone, or the slot was taken.
//	         No row yet: INSERT; a unique-key collision means someone was first.
//	renew    UPDATE gst_leases SET expires_at_ms = :now + 15000
//	          WHERE name = :name AND holder = :holder AND expires_at_ms > :now
//	         0 rows: lost.
//	verify   SELECT term FROM gst_leases WHERE name = :name AND holder = :holder
//	         Run as the first statement of every transaction under the lease;
//	         0 rows: lost, and not one business statement runs.
//	release  UPDATE gst_leases SET expires_at_ms = 0
//	          WHERE name = :name AND holder = :holder
//	         0 rows: the lease was already gone; the work is done either way.
//
// :now is the database server's clock in UTC milliseconds — MySQL
// UNIX_TIMESTAMP(NOW(3)), PostgreSQL clock_timestamp(), SQLite unixepoch() —
// so the processes of a deployment need not agree on the time. A ClickHouse
// primary database has none of this: the claim fails, and with it whatever
// capability asked for the lease.
//
// The holder renews every 5 seconds and gives itself up 10 seconds after the
// last renewal it started, 5 seconds before the database lets anyone else
// claim the name: a holder that cannot reach the database stops before its
// successor can start. The context Hold returns ends at that moment, and the
// transactions opened under it end with it — the standard library rolls back
// a transaction whose context ends and refuses its Commit. Verify is the
// third line, for the transaction that would open after the loss.
//
// Importing the package is what brings leases into a process: the table
// joins the registered models and the transaction guard is installed from
// init, so a project that links no capability built on leases carries no
// table, no statement and no goroutine.
package lease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/instance"
	"github.com/hydroan/gst/internal/modelregistry"
	"gorm.io/gorm"
)

// The protocol's timings, in the proportions of client-go's leader election:
// a holder that has not renewed for localDeadline gives up 5 seconds before
// the database would let anyone else claim the name. Variables so a test can
// play the protocol out in milliseconds.
var (
	// leaseDuration is how long a claim or renewal holds the name, by the
	// database clock.
	leaseDuration = 15 * time.Second
	// renewInterval is how often a holder renews.
	renewInterval = 5 * time.Second
	// localDeadline is how long a holder keeps going without a successful
	// renewal, counted from the moment the last successful one started.
	localDeadline = 10 * time.Second
)

// table is the name of the lease table.
const table = "gst_leases"

var (
	// ErrLost reports that the lease is no longer held: it expired and
	// someone else claimed the name, or it was released. Work under the
	// lease stops on it.
	ErrLost = errors.New("lease lost")
	// ErrUnsupportedDatabase reports a primary database the protocol cannot
	// run on: ClickHouse has neither the row-level updates nor the unique
	// keys a lease needs.
	ErrUnsupportedDatabase = errors.New("leases need a MySQL, PostgreSQL or SQLite primary database")
)

// row is one coordinated name in gst_leases.
type row struct {
	modelregistry.AutoBase
	Name        string `gorm:"size:191;not null"`  // "cron:<job>"; each capability prefixes its own names
	Holder      string `gorm:"size:32;not null"`   // token minted per claim, never reused
	Instance    string `gorm:"size:191;not null"`  // the process holding it; for reading logs only
	Term        uint64 `gorm:"not null;default:0"` // +1 every time the name changes hands
	ExpiresAtMs int64  `gorm:"not null;default:0"` // database clock, UTC milliseconds
	SlotMs      int64  `gorm:"not null;default:0"` // scheduler only: the last instant claimed, never decreasing
}

func (*row) TableName() string { return table }
func (*row) Purge() bool       { return true }

// Indexes declares the name unique: one row per coordinated name is what
// makes a claim's INSERT lose to whoever inserted first.
func (*row) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Name"}, Unique: true}}
}

func init() {
	// Importing this package brings the table and the transaction guard: a
	// project that links no capability built on leases carries neither.
	modelregistry.RegisterTable[*row]()
	dbruntime.SetTransactionGuard(Verify)
}

// Handle is a claimed lease: the proof of holding a name from the claim until
// the release, or the loss.
type Handle struct {
	name   string
	holder string
	term   uint64
	slotMs int64
}

// Name returns the coordinated name.
func (h *Handle) Name() string { return h.name }

// Term returns the term the claim started: the number the world outside the
// database can refuse stale holders by.
func (h *Handle) Term() uint64 { return h.term }

// Available reports whether the primary database can carry leases: nil on
// MySQL, PostgreSQL and SQLite, ErrUnsupportedDatabase on ClickHouse, and an
// error before the database is initialized. A capability built on leases
// checks it as it starts, so a deployment that cannot coordinate fails at
// startup instead of running every replica as if it were alone.
func Available() error {
	_, _, err := primary()
	return err
}

// Claim tries to take name for this process, once, without waiting: it
// returns the handle and true when the name was free — never claimed, expired
// or released — and false when someone holds it. An error means the database
// could not answer.
func Claim(ctx context.Context, name string) (*Handle, bool, error) {
	return claim(ctx, name, nil)
}

// LastSlot returns the last instant claimed under name and whether the name
// has ever been claimed. The scheduler reads it as it starts, to tell an
// instant no replica ran from a job that has never run at all.
func LastSlot(ctx context.Context, name string) (time.Time, bool, error) {
	db, _, err := primary()
	if err != nil {
		return time.Time{}, false, err
	}
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

// ClaimSlot is Claim for the scheduler: it also requires the instant slot to
// lie after the last instant claimed under name, and records it. A cluster
// then runs each instant of a job once, whichever replica gets there first,
// and an instant already run is refused everywhere.
func ClaimSlot(ctx context.Context, name string, slot time.Time) (*Handle, bool, error) {
	slotMs := slot.UnixMilli()
	return claim(ctx, name, &slotMs)
}

// claim runs the claim statement, and the insert behind it for a name the
// table has never seen.
func claim(ctx context.Context, name string, slotMs *int64) (*Handle, bool, error) {
	db, now, err := primary()
	if err != nil {
		return nil, false, err
	}
	holder, err := newHolder()
	if err != nil {
		return nil, false, err
	}
	updatedAt := dbruntime.NowUTC()

	var setSlot, whereSlot string
	args := []any{holder, instance.ID(), leaseDuration.Milliseconds(), updatedAt}
	if slotMs != nil {
		setSlot = ", slot_ms = ?"
		args = append(args, *slotMs)
	}
	args = append(args, name)
	if slotMs != nil {
		whereSlot = " AND slot_ms < ?"
		args = append(args, *slotMs)
	}
	update := fmt.Sprintf("UPDATE %s SET holder = ?, instance = ?, term = term + 1, expires_at_ms = %s + ?, updated_at = ?%s WHERE name = ? AND expires_at_ms <= %s%s",
		table, now, setSlot, now, whereSlot)
	res := db.WithContext(ctx).Exec(update, args...)
	if res.Error != nil {
		return nil, false, errors.Wrapf(res.Error, "claim lease %q", name)
	}
	if res.RowsAffected == 1 {
		return handleOf(ctx, db, name, holder)
	}

	// No row was free. Either the name is held — the insert then collides
	// with it — or the table has never seen the name and the insert is the
	// claim; losing the insert to another process means it was first.
	var slotValue int64
	if slotMs != nil {
		slotValue = *slotMs
	}
	insert := fmt.Sprintf("INSERT INTO %s (name, holder, instance, term, expires_at_ms, slot_ms, created_at, updated_at) VALUES (?, ?, ?, 1, %s + ?, ?, ?, ?)", table, now)
	res = db.WithContext(ctx).Exec(insert, name, holder, instance.ID(), leaseDuration.Milliseconds(), slotValue, updatedAt, updatedAt)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
			return nil, false, nil
		}
		return nil, false, errors.Wrapf(res.Error, "claim lease %q", name)
	}
	return &Handle{name: name, holder: holder, term: 1, slotMs: slotValue}, true, nil
}

// handleOf reads the term the claim just started; the update that won the
// name does not report it.
func handleOf(ctx context.Context, db *gorm.DB, name, holder string) (*Handle, bool, error) {
	var claimed struct {
		Term   uint64
		SlotMs int64
	}
	res := db.WithContext(ctx).Raw(fmt.Sprintf("SELECT term, slot_ms FROM %s WHERE name = ? AND holder = ?", table), name, holder).Scan(&claimed)
	if res.Error != nil {
		return nil, false, errors.Wrapf(res.Error, "read the term of lease %q", name)
	}
	if res.RowsAffected == 0 {
		// Claimed and gone within the same instant: only a release or an
		// expiry could do that, neither of which this holder has done.
		return nil, false, errors.Wrapf(ErrLost, "lease %q vanished right after the claim", name)
	}
	return &Handle{name: name, holder: holder, term: claimed.Term, slotMs: claimed.SlotMs}, true, nil
}

// Renew extends the lease by leaseDuration from the database's now. ErrLost
// reports the lease expired and was claimed by someone else, or was
// released; any other error means the database could not answer, which is
// not yet a loss — Hold keeps trying until the local deadline.
func (h *Handle) Renew(ctx context.Context) error {
	db, now, err := primary()
	if err != nil {
		return err
	}
	res := db.WithContext(ctx).Exec(
		fmt.Sprintf("UPDATE %s SET expires_at_ms = %s + ?, updated_at = ? WHERE name = ? AND holder = ? AND expires_at_ms > %s", table, now, now),
		leaseDuration.Milliseconds(), dbruntime.NowUTC(), h.name, h.holder)
	if res.Error != nil {
		return errors.Wrapf(res.Error, "renew lease %q", h.name)
	}
	if res.RowsAffected == 0 {
		return errors.Wrapf(ErrLost, "renew lease %q", h.name)
	}
	return nil
}

// Release gives the name up at once instead of letting the lease run out,
// so the next claimant need not wait. ErrLost reports the lease was already
// gone; the work it protected is done either way, so a caller logs it and
// moves on.
func (h *Handle) Release(ctx context.Context) error {
	db, _, err := primary()
	if err != nil {
		return err
	}
	res := db.WithContext(ctx).Exec(
		fmt.Sprintf("UPDATE %s SET expires_at_ms = 0, updated_at = ? WHERE name = ? AND holder = ?", table),
		dbruntime.NowUTC(), h.name, h.holder)
	if res.Error != nil {
		return errors.Wrapf(res.Error, "release lease %q", h.name)
	}
	if res.RowsAffected == 0 {
		return errors.Wrapf(ErrLost, "release lease %q", h.name)
	}
	return nil
}

// Verify is the transaction guard: run on tx as the first statement of a
// transaction opened under a lease, it reports ErrLost when the lease is no
// longer this holder's, so not one business statement of the transaction
// runs. A context carrying no lease passes. It is the third line behind the
// holder's own deadline and the context cancellation, for the transaction
// that would open after the loss.
func Verify(ctx context.Context, tx *gorm.DB) error {
	h, ok := FromContext(ctx)
	if !ok {
		return nil
	}
	var term uint64
	res := tx.Raw(fmt.Sprintf("SELECT term FROM %s WHERE name = ? AND holder = ?", table), h.name, h.holder).Scan(&term)
	if res.Error != nil {
		return errors.Wrapf(res.Error, "verify lease %q", h.name)
	}
	if res.RowsAffected == 0 {
		return errors.Wrapf(ErrLost, "verify lease %q", h.name)
	}
	return nil
}

// handleKey carries the lease the work on a context runs under.
type handleKey struct{}

// WithHandle returns a context carrying h: the work on it runs under the
// lease, its transactions verify the lease first, and TermFromContext finds
// the term.
func WithHandle(ctx context.Context, h *Handle) context.Context {
	return context.WithValue(ctx, handleKey{}, h)
}

// FromContext returns the lease ctx carries, if any.
func FromContext(ctx context.Context) (*Handle, bool) {
	h, ok := ctx.Value(handleKey{}).(*Handle)
	return h, ok && h != nil
}

// TermFromContext returns the term of the lease ctx carries, and whether it
// carries one.
func TermFromContext(ctx context.Context) (uint64, bool) {
	h, ok := FromContext(ctx)
	if !ok {
		return 0, false
	}
	return h.term, true
}

// primary returns the primary database and the expression for its clock in
// UTC milliseconds — the only clock the protocol reads, so the processes of
// a deployment need not agree on the time.
func primary() (*gorm.DB, string, error) {
	db := dbruntime.DB
	if db == nil {
		return nil, "", errors.New("lease: the database is not initialized")
	}
	now, err := nowExpression(dialectOf(db))
	if err != nil {
		return nil, "", err
	}
	return db, now, nil
}

// dialectOf names the dialect of a connection handle, in the gorm driver's
// spelling.
func dialectOf(db *gorm.DB) string {
	if db == nil || db.Dialector == nil {
		return ""
	}
	return strings.ToLower(db.Dialector.Name())
}

// nowExpression returns the SQL that reads the database server's clock in
// UTC milliseconds for dialect, or ErrUnsupportedDatabase.
func nowExpression(dialect string) (string, error) {
	switch dialect {
	case "mysql":
		return "FLOOR(UNIX_TIMESTAMP(NOW(3)) * 1000)", nil
	case "postgres":
		return "(EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::BIGINT", nil
	case "sqlite":
		return "CAST(unixepoch('subsec') * 1000 AS INTEGER)", nil
	default:
		return "", errors.Wrapf(ErrUnsupportedDatabase, "primary database %q", dialect)
	}
}

// newHolder mints the token a claim is known by: 16 random bytes as hex,
// never reused, so "is it still mine" is a question about this claim and not
// about this process.
func newHolder() (string, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return "", errors.Wrap(err, "mint a lease holder token")
	}
	return hex.EncodeToString(token), nil
}
