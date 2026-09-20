// Package lease is the coordination primitive the framework's distributed
// capabilities stand on: a name that at most one healthy process among those
// sharing the primary database holds at a time. The scheduler claims a name
// per job and instant, so a cluster claims each instant once, and a second
// time only for a round cut short; a capability that keeps a name for as
// long as its holder stays healthy, or claims one for the duration of a
// call, is built the same way.
//
// A lease is one row of gst_leases, updated in place: who holds the name
// (holder, a token minted per claim and never reused), which process that is
// (instance, for reading logs), the term (incremented every time the name
// changes hands, and logged with every round and tenure) and when the lease
// expires (by the database clock, never a process clock).
// Four single-row statements do all the work; no lock outlives its own
// statement:
//
//	claim    UPDATE gst_leases SET holder = :holder, instance = :instance,
//	             term = term + 1, expires_at_ms = :now + 15000
//	          WHERE name = :name AND expires_at_ms <= :now
//	         1 row: claimed. 0 rows: held by someone.
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
// The scheduler claims a name per job and instant, and keeps three columns
// more on it: the last instant claimed (slot_ms, never decreasing), that
// instant again for as long as its round has not run to its end
// (unfinished_slot_ms, 0 once it has) and the last instant claimed a second
// time (rerun_slot_ms). A round cut short — its process shut down or died,
// or its lease was lost — thus stays on record, and runs again once. Only
// the last instant claimed can be unfinished: an unfinished_slot_ms other
// than slot_ms is settled. The scheduler's statements take the place of
// claim, and of release for a round that ran to its end — a round cut short
// gives its lease back with release — and add three:
//
//	claim an instant
//	         SELECT term, slot_ms, unfinished_slot_ms, rerun_slot_ms,
//	             expires_at_ms <= :now FROM gst_leases WHERE name = :name
//	         then, when the lease expired and :slot is past slot_ms:
//	         UPDATE gst_leases SET holder = :holder, instance = :instance,
//	             term = term + 1, expires_at_ms = :now + 15000,
//	             slot_ms = :slot, unfinished_slot_ms = :slot
//	          WHERE name = :name AND term = :term AND expires_at_ms <= :now
//	         1 row: claimed, and an unfinished_slot_ms read equal to the
//	         slot_ms read is an instant given up for this one. 0 rows:
//	         someone claimed first. No row yet: the INSERT, with both slots.
//	finish   UPDATE gst_leases SET expires_at_ms = 0, unfinished_slot_ms = 0
//	          WHERE name = :name AND holder = :holder
//	         the release of a round that ran to its end. 0 rows — the lease
//	         was lost meanwhile — records the end without the lease:
//	         UPDATE gst_leases SET unfinished_slot_ms = 0
//	          WHERE name = :name AND unfinished_slot_ms = :slot
//	find     SELECT name, term, slot_ms FROM gst_leases
//	          WHERE name IN (:names) AND unfinished_slot_ms = slot_ms
//	            AND rerun_slot_ms < slot_ms AND expires_at_ms <= :now
//	         the instants whose round was cut short and that are free to
//	         claim again, for every job of a scheduler at once.
//	claim again
//	         UPDATE gst_leases SET holder = :holder, instance = :instance,
//	             term = term + 1, expires_at_ms = :now + 15000,
//	             rerun_slot_ms = slot_ms
//	          WHERE name = :name AND term = :term AND unfinished_slot_ms = slot_ms
//	            AND rerun_slot_ms < slot_ms AND expires_at_ms <= :now
//	         1 row: claimed; the term found holds the row to the instant
//	         found. 0 rows: claimed again elsewhere, recorded finished, or
//	         given up for a later instant.
//	last     SELECT slot_ms FROM gst_leases WHERE name = :name
//	         read as the scheduler starts, to tell an instant no replica
//	         claimed from a job that has never run.
//
// The instant claims read before they write because the claim of an instant
// has to know what it gives up. A term compared on the write makes the pair
// as exclusive as a single statement: every claim moves the term, so of
// replicas that read the same row, one write matches.
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
	"sync"
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
// let anyone else claim the name. Variables rather than constants so the
// framework's own tests can play the protocol out in milliseconds, which is
// what SetTimings in testhooks.go is for.
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

// statementFloor is the least a statement of the protocol is given, whatever
// the protocol's own timings are. The bound exists for a connection that is
// gone but not closed, which answers nothing at all; a database answering in
// under a second is not that case, and a test playing the protocol out in
// milliseconds must not turn an ordinary round trip into a failure.
const statementFloor = time.Second

// bounded returns ctx with the bound one statement of the protocol gets when
// its caller has none of its own: a claim, the read of the last instant, the
// sweep for the rounds cut short. A database that stopped answering would
// otherwise hold whoever asked — a scheduler loop, a campaign, the sweep —
// for as long as it stays silent, which on a connection that is gone but not
// closed is forever. Half the local deadline is what a renewal attempt gets
// (see Hold): a statement that cannot answer within it would not have kept a
// lease alive either, and the caller comes back at its own cadence.
func bounded(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, max(localDeadline/2, statementFloor))
}

// RetryInterval is how soon a caller tries again after the database failed
// to answer: the cadence the protocol already keeps against it, which a
// caller waiting for the same database has no reason to beat.
func RetryInterval() time.Duration {
	return renewInterval
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
//
// unfinished_slot_ms holds the instant of a round that has not run to its
// end, and 0 once it has, rather than the last instant that did: a row
// holding the column's default has nothing left to run again. Only the last
// instant claimed can be unfinished, so a value other than slot_ms reads as
// settled too.
type row struct {
	modelregistry.AutoBase
	Name             string `gorm:"size:191;not null"`  // "cron:<job>"; each capability prefixes its own names, within nameMaxLength
	Holder           string `gorm:"size:32;not null"`   // token minted per claim, never reused
	Instance         string `gorm:"size:191;not null"`  // the process holding it; for reading logs only
	Term             uint64 `gorm:"not null;default:0"` // +1 every time the name changes hands
	ExpiresAtMs      int64  `gorm:"not null;default:0"` // database clock, UTC milliseconds
	SlotMs           int64  `gorm:"not null;default:0"` // scheduler only: the last instant claimed, never decreasing
	UnfinishedSlotMs int64  `gorm:"not null;default:0"` // scheduler only: slot_ms until its round has run to its end, then 0
	RerunSlotMs      int64  `gorm:"not null;default:0"` // scheduler only: the last instant claimed a second time; none is claimed a third
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
	// slotMs is the instant a scheduler's claim was for, 0 for any other
	// claim.
	slotMs int64
	// superseded is the instant a scheduler's claim gave up, see Superseded.
	superseded *Unfinished
	// claimedAt is the moment the claim was sent, on this process's clock:
	// the first renewal the local deadline counts from. Taken before the
	// statement, like every renewal's, so that the round trip of the claim
	// itself does not eat into the margin between the deadline and the
	// database's expiry.
	claimedAt time.Time
	// lost closes the moment Hold knows the lease is lost, whatever else has
	// ended the work's context by then; see Lost.
	lost     chan struct{}
	lostOnce sync.Once
}

// newHandle returns the handle of the claim of name by holder that started
// term, sent at claimedAt.
func newHandle(name, holder string, term uint64, claimedAt time.Time) *Handle {
	return &Handle{name: name, holder: holder, term: term, claimedAt: claimedAt, lost: make(chan struct{})}
}

// Name returns the coordinated name.
func (h *Handle) Name() string { return h.name }

// Term returns the term the claim started: the number the world outside the
// database can refuse stale holders by.
func (h *Handle) Term() uint64 { return h.term }

// Lost reports whether the lease is known lost — a renewal found it gone, or
// none succeeded before the local deadline — whether or not the context of
// the work under it had already ended for another reason, such as the
// process shutting down. Only Hold learns of a loss, so a handle never held
// is never lost.
func (h *Handle) Lost() bool {
	select {
	case <-h.lost:
		return true
	default:
		return false
	}
}

// markLost records the lease as lost; see Lost.
func (h *Handle) markLost() {
	h.lostOnce.Do(func() { close(h.lost) })
}

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
	ctx, cancel := bounded(ctx)
	defer cancel()

	claimedAt := time.Now()
	res := db.WithContext(ctx).Exec(
		fmt.Sprintf("UPDATE %s SET holder = ?, instance = ?, term = term + 1, expires_at_ms = %s + ?, updated_at = ? WHERE name = ? AND expires_at_ms <= %s", table, now, now),
		holder, instance.ID(), leaseDuration.Milliseconds(), updatedAt, name)
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
	return insertClaim(ctx, db, now, name, holder, 0, updatedAt, claimedAt)
}

// insertClaim runs the insert that claims a name the table has never seen,
// for the instant slotMs, or none when it is 0, and returns the handle and
// true when it inserted the row.
func insertClaim(ctx context.Context, db *gorm.DB, now, name, holder string, slotMs int64, updatedAt, claimedAt time.Time) (*Handle, bool, error) {
	res := db.WithContext(ctx).Exec(claimInsert(dialectOf(db), now), name, holder, instance.ID(), leaseDuration.Milliseconds(), slotMs, slotMs, updatedAt, updatedAt)
	if res.Error != nil {
		return nil, false, errors.Wrapf(res.Error, "claim lease %q", name)
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	h := newHandle(name, holder, 1, claimedAt)
	h.slotMs = slotMs
	return h, true, nil
}

// claimInsert returns the insert that claims a name the table has never
// seen, worded so that finding the name there — inserted by another process
// a moment earlier, or held for a long time — inserts nothing instead of
// failing on the unique key: INSERT IGNORE on MySQL, ON CONFLICT DO NOTHING
// on PostgreSQL and SQLite. The scheduler's insert records its instant as the
// last one claimed and as unfinished; any other records none.
//
// INSERT IGNORE also turns a value too long for its column into a warning
// and a truncated row, which is why every name is validated before it gets
// here: for the literal values the statement carries nothing else IGNORE
// would hide can occur. ON DUPLICATE KEY UPDATE name = name is no
// alternative: the connection sets CLIENT_FOUND_ROWS, under which a no-op
// update reports one affected row, the same as an insert.
func claimInsert(dialect, now string) string {
	columns := fmt.Sprintf("(name, holder, instance, term, expires_at_ms, slot_ms, unfinished_slot_ms, created_at, updated_at) VALUES (?, ?, ?, 1, %s + ?, ?, ?, ?, ?)", now)
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
	return newHandle(name, holder, term, claimedAt), true, nil
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
