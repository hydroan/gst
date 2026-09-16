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
//	         No row yet: INSERT, worded to do nothing when the name is there
//	         (INSERT IGNORE, ON CONFLICT DO NOTHING); 0 rows: someone was first.
//	renew    UPDATE gst_leases SET expires_at_ms = :now + 15000
//	          WHERE name = :name AND holder = :holder AND expires_at_ms > :now
//	         0 rows: lost.
//	verify   SELECT term FROM gst_leases
//	          WHERE name = :name AND holder = :holder AND expires_at_ms > :now
//	         Run as the first statement of every database.Transaction opened
//	         under the lease; 0 rows: lost, and not one business statement
//	         runs. Writes outside such a transaction are not checked: like
//	         a Kubernetes leader, work stops through its context, not
//	         through a check on every statement.
//	release  UPDATE gst_leases SET expires_at_ms = 0
//	          WHERE name = :name AND holder = :holder
//	         0 rows: the lease was already gone; the work is done either way.
//
// :now is the database server's clock in UTC milliseconds — MySQL
// UNIX_TIMESTAMP() with the milliseconds of NOW(3), PostgreSQL
// clock_timestamp(), SQLite unixepoch() — read without a round trip through
// any session time zone, so the processes of a deployment need not agree on
// the time. A ClickHouse primary database has none of this: the claim
// fails, and with it whatever capability asked for the lease.
//
// The holder renews every 2 seconds, a renewal that failed included, each
// attempt waiting at most 5 seconds, and gives itself up 10 seconds after the
// last successful renewal started, 5 seconds before the database lets anyone
// else claim the name: a database that stalls or drops a statement for a few
// seconds costs the holder nothing, and a holder that cannot reach it stops
// before its successor can start. The context Hold returns ends at that
// moment, and the transactions opened under it end with it — the standard
// library rolls back a transaction whose context ends and refuses its
// Commit. Verify is the third line, for the transaction that would open
// after the loss, and Run the last: work that ignores all three and runs on
// after the loss fails the process, which then ends without waiting for it.
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

// The protocol's timings, those of client-go's leader election: a holder
// renews at the retry period and keeps retrying a renewal that failed until
// localDeadline has passed, then gives up 5 seconds before the database would
// let anyone else claim the name. Variables so a test can play the protocol
// out in milliseconds.
var (
	// leaseDuration is how long a claim or renewal holds the name, by the
	// database clock.
	leaseDuration = 15 * time.Second
	// renewInterval is how often a holder renews, and retries a renewal that
	// failed, counted from the start of the previous attempt.
	renewInterval = 2 * time.Second
	// localDeadline is how long a holder keeps going without a successful
	// renewal, counted from the moment the last successful one started.
	localDeadline = 10 * time.Second
	// stepDownGrace is how long work may take to return once its lease is
	// lost before the process fails, see Run.
	stepDownGrace = 5 * time.Second
)

// SetTimings replaces the protocol's timings and returns the function that
// restores them. It exists for the tests of the capabilities built on leases,
// which play the protocol out in milliseconds; a process runs the one
// protocol every process of its deployment agrees on, so nothing else calls
// it.
func SetTimings(lease, renew, deadline, grace time.Duration) (restore func()) {
	originalLease, originalRenew, originalDeadline, originalGrace := leaseDuration, renewInterval, localDeadline, stepDownGrace
	leaseDuration, renewInterval, localDeadline, stepDownGrace = lease, renew, deadline, grace
	return func() {
		leaseDuration, renewInterval, localDeadline, stepDownGrace = originalLease, originalRenew, originalDeadline, originalGrace
	}
}

// table is the name of the lease table.
const table = "gst_leases"

// nameMaxLength is the most bytes a coordinated name may have: the width of
// the name column, 191 characters, which hold at least that many bytes.
const nameMaxLength = 191

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

// row is one coordinated name in gst_leases. The protocol reads one clock,
// the database's, and it reads it through expires_at_ms alone; created_at
// and updated_at are the framework's columns, stamped with the writing
// process's clock, and are there for reading the table, not for deciding.
type row struct {
	modelregistry.AutoBase
	Name        string `gorm:"size:191;not null"`  // "cron:<job>"; each capability prefixes its own names, within nameMaxLength
	Holder      string `gorm:"size:32;not null"`   // token minted per claim, never reused
	Instance    string `gorm:"size:191;not null"`  // the process holding it; for reading logs only
	Term        uint64 `gorm:"not null;default:0"` // +1 every time the name changes hands
	ExpiresAtMs int64  `gorm:"not null;default:0"` // database clock, UTC milliseconds
	SlotMs      int64  `gorm:"not null;default:0"` // scheduler only: the last instant claimed, never decreasing
}

func (*row) TableName() string { return table }
func (*row) Purge() bool       { return true }

// Indexes declares the name unique: one row per coordinated name is what
// makes a claim's INSERT insert nothing when another process inserted first.
func (*row) Indexes() []modelregistry.Index {
	return []modelregistry.Index{{Fields: []string{"Name"}, Unique: true}}
}

func init() {
	// Importing this package brings the table and the transaction guard: a
	// project that links no capability built on leases carries neither.
	modelregistry.RegisterTable[*row]()
	dbruntime.SetTransactionGuard(Verify)
}

// Available reports whether the primary database can carry leases: nil on
// MySQL, PostgreSQL and SQLite, ErrUnsupportedDatabase on ClickHouse, and an
// error before the database is initialized. A capability built on leases
// checks it as it starts, so a deployment that cannot coordinate fails at
// startup instead of running every replica as if it were alone.
func Available() error {
	_, _, err := primary()
	return err
}

// ValidateName reports whether name can be a coordinated name: not empty, and
// within the width of the name column. The capabilities validate the names
// they register at startup, so a name that could never be claimed fails the
// process instead of leaving work that silently never runs — MySQL would
// truncate the name on the claim's insert and PostgreSQL refuse it, every
// time — and claim checks again, so no path reaches the table with a name it
// cannot hold.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("lease: empty name")
	}
	if len(name) > nameMaxLength {
		return errors.Newf("lease: name %q is %d bytes, longer than the %d the name column holds", name, len(name), nameMaxLength)
	}
	return nil
}

// Handle is a claimed lease: the proof of holding a name from the claim until
// the release, or the loss.
type Handle struct {
	name   string
	holder string
	term   uint64
	// claimedAt is the moment the claim was sent, on this process's clock:
	// the first renewal the local deadline counts from. Taken before the
	// statement, like every renewal's, so that the round trip of the claim
	// itself does not eat into the margin between the deadline and the
	// database's expiry.
	claimedAt time.Time
}

// Name returns the coordinated name.
func (h *Handle) Name() string { return h.name }

// Term returns the term the claim started: the number the world outside the
// database can refuse stale holders by.
func (h *Handle) Term() uint64 { return h.term }

// Renew extends the lease by leaseDuration from the database's now. ErrLost
// reports the lease expired — claimed by someone else since, or not yet,
// which the holder's own deadline makes unreachable in normal operation —
// or was released; any other error means the database could not answer,
// which is not yet a loss — Hold keeps trying until the local deadline.
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

// Claim tries to take name for this process, once, without waiting: it
// returns the handle and true when the name was free — never claimed, expired
// or released — and false when someone holds it. An error means the database
// could not answer.
func Claim(ctx context.Context, name string) (*Handle, bool, error) {
	return claim(ctx, name, nil)
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
	claimedAt := time.Now()
	res := db.WithContext(ctx).Exec(update, args...)
	if res.Error != nil {
		return nil, false, errors.Wrapf(res.Error, "claim lease %q", name)
	}
	if res.RowsAffected == 1 {
		return handleOf(ctx, db, name, holder, claimedAt)
	}

	// No row was free. Either the name is held — the insert then does
	// nothing — or the table has never seen the name and the insert is the
	// claim; inserting nothing because another process inserted first means
	// it was first. Neither is an error, and neither reaches the SQL log as
	// one: the refused claims of every replica are the protocol's normal
	// traffic.
	var slotValue int64
	if slotMs != nil {
		slotValue = *slotMs
	}
	res = db.WithContext(ctx).Exec(claimInsert(dialectOf(db), now), name, holder, instance.ID(), leaseDuration.Milliseconds(), slotValue, updatedAt, updatedAt)
	if res.Error != nil {
		return nil, false, errors.Wrapf(res.Error, "claim lease %q", name)
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	return &Handle{name: name, holder: holder, term: 1, claimedAt: claimedAt}, true, nil
}

// claimInsert returns the insert that claims a name the table has never
// seen, worded so that finding the name there — inserted by another process
// a moment earlier, or held for a long time — inserts nothing instead of
// failing on the unique key: INSERT IGNORE on MySQL, ON CONFLICT DO NOTHING
// on PostgreSQL and SQLite.
//
// INSERT IGNORE also turns a value too long for its column into a warning
// and a truncated row, which is why every name is validated before it gets
// here: for the literal values the statement carries nothing else IGNORE
// would hide can occur. ON DUPLICATE KEY UPDATE name = name is no
// alternative: the connection sets CLIENT_FOUND_ROWS, under which a no-op
// update reports one affected row, the same as an insert.
func claimInsert(dialect, now string) string {
	columns := fmt.Sprintf("(name, holder, instance, term, expires_at_ms, slot_ms, created_at, updated_at) VALUES (?, ?, ?, 1, %s + ?, ?, ?, ?)", now)
	if dialect == "mysql" {
		return fmt.Sprintf("INSERT IGNORE INTO %s %s", table, columns)
	}
	return fmt.Sprintf("INSERT INTO %s %s ON CONFLICT (name) DO NOTHING", table, columns)
}

// handleOf reads the term the claim just started; the update that won the
// name does not report it.
func handleOf(ctx context.Context, db *gorm.DB, name, holder string, claimedAt time.Time) (*Handle, bool, error) {
	var term uint64
	res := db.WithContext(ctx).Raw(fmt.Sprintf("SELECT term FROM %s WHERE name = ? AND holder = ?", table), name, holder).Scan(&term)
	if res.Error != nil {
		return nil, false, errors.Wrapf(res.Error, "read the term of lease %q", name)
	}
	if res.RowsAffected == 0 {
		// Claimed and gone within the same instant: only a release or an
		// expiry could do that, neither of which this holder has done.
		return nil, false, errors.Wrapf(ErrLost, "lease %q vanished right after the claim", name)
	}
	return &Handle{name: name, holder: holder, term: term, claimedAt: claimedAt}, true, nil
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

// Verify is the transaction guard: run as the first statement of a
// transaction opened under a lease, it reports ErrLost when the lease is no
// longer this holder's — taken by another, released, or expired with no
// one to take it yet — so not one business statement of the transaction
// runs. A context carrying no lease passes. It is the third line behind the
// holder's own deadline and the context cancellation, for the transaction
// that would open after the loss.
//
// The lease rows live on the primary database, so a transaction opened
// there runs the check on itself, and one opened on another instance —
// which has no lease table — is checked against the primary instead, still
// before its first statement.
func Verify(ctx context.Context, base, tx *gorm.DB) error {
	h, ok := FromContext(ctx)
	if !ok {
		return nil
	}
	db, now, err := primary()
	if err != nil {
		return err
	}
	conn := tx
	if base == nil || base.ConnPool != db.ConnPool {
		conn = db.WithContext(ctx)
	}
	var term uint64
	res := conn.Raw(fmt.Sprintf("SELECT term FROM %s WHERE name = ? AND holder = ? AND expires_at_ms > %s", table, now), h.name, h.holder).Scan(&term)
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
// lease, the database.Transaction calls it makes verify the lease first, and
// TermFromContext finds the term.
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
		// UNIX_TIMESTAMP() without an argument reads the epoch as is; with
		// NOW(3) as its argument it would convert a wall-clock time back
		// through the session time zone, which is ambiguous for the hour a
		// daylight-saving zone repeats. The milliseconds come from NOW(3),
		// read at the same statement start.
		return "UNIX_TIMESTAMP() * 1000 + MICROSECOND(NOW(3)) DIV 1000", nil
	case "postgres":
		return "FLOOR(EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::BIGINT", nil
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
