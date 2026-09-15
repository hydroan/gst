package dbruntime

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"hash/fnv"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// tablePreparationStarted marks that InitDatabase has started the goroutine
// that prepares tables, so a second call does not start a second one and Wait
// can tell a queue being drained from one nothing is draining.
var tablePreparationStarted atomic.Int32

// prepareTable creates the table backing one queued model in the default
// database, and reports the model done only once the table is there. Wait
// counts on that ordering: a model still counts as pending for the whole of
// ensureTable, not just for the time it sits on the queue.
func prepareTable(db *gorm.DB, m types.Model) {
	defer modelregistry.TableDone()

	// Touching the version metadata here makes a defective model.Version
	// declaration (embedded, or missing its required tag) fail at startup
	// for every registered model, instead of on its first write.
	modelregistry.IsVersioned(m)

	begin := time.Now()
	typ := reflect.TypeOf(m).Elem()
	if err := ensureTable(db, m); err != nil {
		panic(errors.Wrap(err, fmt.Sprintf("failed to prepare table(%s)", typ.String())))
	}
	zap.S().Infow("database table ready", "model", typ.String(), util.LogDuration(time.Since(begin)))
}

// ensureTable prepares the backing table for a registered model.
//
// With database.auto_migrate enabled it runs gorm AutoMigrate and creates
// custom indexes, which suits local development and tests. With the option
// disabled (the default) it only verifies that the table already exists via
// the dialect-aware gorm Migrator, so schema changes in shared environments
// stay an explicit "gg migrate" decision instead of a startup side effect.
//
// An in-memory sqlite database is exempt from that check: it is created empty
// in every process and dies with it, so no earlier "gg migrate" run can have
// populated it and there is no shared schema to protect. Migrating it anyway
// keeps the zero-config defaults (sqlite, in-memory, auto_migrate off) bootable
// instead of panicking on the first registered model.
func ensureTable(handler *gorm.DB, m types.Model) error {
	tableName, err := requireTableName(m)
	if err != nil {
		return err
	}

	// ClickHouse schema — engine, ORDER BY, partitioning, TTL — is a
	// query-model design the framework cannot derive from a Go struct, so it
	// is never created or migrated here: the application owns it through
	// hand-written DDL, and bootstrap only verifies the table exists.
	if handler.Dialector != nil && strings.ToLower(handler.Dialector.Name()) == "clickhouse" {
		if !handler.Migrator().HasTable(tableName) {
			return errors.Newf("table %q does not exist: clickhouse tables are managed by hand-written DDL on the application side, create it before starting", tableName)
		}
		return nil
	}

	inMemory := config.App.Database.Type == config.DBSqlite && config.App.Sqlite.IsMemory
	if config.App.Database.AutoMigrate || inMemory {
		return migrateTable(handler, m, tableName)
	}
	if !handler.Migrator().HasTable(tableName) {
		return errors.Newf("table %q does not exist: run \"gg migrate\" to apply the schema, or enable database.auto_migrate for local development", tableName)
	}
	return nil
}

// migrateTable runs gorm AutoMigrate — on a session that leaves the
// framework's indexes alone, see automigrating — and creates the custom
// indexes, once across the processes sharing the database. Replicas
// starting together all find the table missing and all issue CREATE TABLE,
// and the server refuses every one but the first: on MySQL and PostgreSQL
// the processes take turns under the startup lock, and where none can be
// held — SQLite, or a pool of a single connection — a failure while another
// process created the table is retried once, against the table that is
// there now.
//
// AutoMigrate reads the table name through gorm's Tabler, which is the
// model's own TableName method. Supplying it again through Table() would make
// gorm re-parse the schema under a special table name, which renames the
// constraints of associated models.
func migrateTable(handler *gorm.DB, m types.Model, tableName string) error {
	return serialized(context.Background(), handler, "migrate", func() error {
		migrate := func() error {
			if err := automigrating(handler).AutoMigrate(m); err != nil {
				return err
			}
			return ensureCustomIndexes(handler, m)
		}
		err := migrate()
		if err != nil && handler.Migrator().HasTable(tableName) {
			err = migrate()
		}
		return err
	})
}

// Serialized runs fn while holding the startup lock named purpose on the
// primary database, so that of the processes of a deployment starting at
// once only one runs it at a time: the seeding the routes-ready hooks do
// reads before it writes, and replicas doing so together would each find
// nothing and each write. Table preparation holds a lock of its own, one
// table at a time; the two do not exclude each other. A process waits for
// the lock for as long as the holder takes, or until ctx ends — the process
// told to stop while it starts — warning every startupLockWaitReport that
// it is still waiting; see lockStartup for why the wait has no bound of its
// own. ctx bounds the wait only: fn runs to its end once the lock is held.
// Where no lock can be held — SQLite, a pool of a single connection, a
// ClickHouse primary, which has no advisory locks and no transactions or
// unique keys for seeding to count on either — fn runs as is. A process
// with no primary database has nothing to coordinate through and runs fn
// as is too.
func Serialized(ctx context.Context, purpose string, fn func() error) error {
	if DB == nil {
		return fn()
	}
	return serialized(ctx, DB, purpose, fn)
}

// serialized is Serialized on the given handle.
func serialized(ctx context.Context, handler *gorm.DB, purpose string, fn func() error) error {
	unlock, err := lockStartup(ctx, handler, purpose)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

// startupLockName names the advisory lock a startup step holds on the
// servers that offer one: one lock per purpose and database, so that the
// processes of a deployment take the step one process at a time while
// deployments on other databases of the same server go on unhindered. MySQL
// scopes a named lock to the whole server, so the database is part of the
// name — hashed, because MySQL allows a lock name 64 characters at most and
// a database name alone may take them all.
func startupLockName(purpose, database string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(database))
	return fmt.Sprintf("gst:%s:%08x", purpose, h.Sum32())
}

// startupLockKey is the lock as an integer, for the server that keys
// advisory locks by one: a stable hash of the name.
func startupLockKey(name string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int64(h.Sum32())
}

// startupLockWaitReport is how often a process waiting for a startup lock
// says so, so that a replica that is slow to become ready shows why. A
// variable so that tests can shorten it.
var startupLockWaitReport = 30 * time.Second

// startupLockPoll is how often a waiting process asks for the lock again.
// The lock is asked for without waiting on the server and the wait happens
// here, between the tries: a statement that waited on the server for as
// long as the holder takes would be cut by a read timeout the project set
// on its connections, and could not be ended by ctx. A variable so that
// tests can shorten it.
var startupLockPoll = time.Second

// startupLockStatementTimeout bounds every statement of the lock — taking
// the connection it lives on, a try, the release — which take milliseconds
// when the database answers at all.
const startupLockStatementTimeout = time.Minute

// startupLock is the advisory lock of one dialect, tried and released by
// name on the connection that holds it.
type startupLock struct {
	// try asks for the lock without waiting and reports whether it was
	// taken.
	try     func(ctx context.Context, conn *sql.Conn, name string) (bool, error)
	release func(ctx context.Context, conn *sql.Conn, name string) error
}

// startupLocks are the advisory locks the startup steps hold, by dialect.
// MySQL names its locks and answers a try with 1 for the lock taken, 0 for
// held elsewhere and NULL for an error; PostgreSQL keys them by an integer
// and answers a try with a boolean.
var startupLocks = map[string]startupLock{
	"mysql": {
		try: func(ctx context.Context, conn *sql.Conn, name string) (bool, error) {
			var got sql.NullInt64
			if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", name).Scan(&got); err != nil {
				return false, err
			}
			if !got.Valid {
				return false, errors.New("the lock could not be tried")
			}
			return got.Int64 == 1, nil
		},
		release: func(ctx context.Context, conn *sql.Conn, name string) error {
			_, err := conn.ExecContext(ctx, "DO RELEASE_LOCK(?)", name)
			return err
		},
	},
	"postgres": {
		try: func(ctx context.Context, conn *sql.Conn, name string) (bool, error) {
			var taken bool
			if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", startupLockKey(name)).Scan(&taken); err != nil {
				return false, err
			}
			return taken, nil
		},
		release: func(ctx context.Context, conn *sql.Conn, name string) error {
			_, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", startupLockKey(name))
			return err
		},
	},
}

// lockStartup takes the startup lock named purpose and returns the function
// that releases it. Both servers scope the lock to the session, so it is
// held on a connection of its own while the step runs on the pool's other
// connections. A pool of a single connection cannot hold the lock and work
// at once, and SQLite and ClickHouse have no such lock: there nothing is
// locked — for table preparation the retries of migrateTable and
// ensureCustomIndexes cover a race, and none of the three is the shape a
// deployment takes.
//
// The wait for the lock has no bound of its own: how long the holder takes
// — a large seeding, a column added to a large table — is the project's,
// and a bound the framework picked would turn a slow start into failed
// ones. The wait cannot outlive the holder: the lock is the holder's
// session, and a holder that crashes drops it with its connection — which
// is also why the process must reach the primary directly or through a
// session-level pool: a proxy that hands a session's statements to
// different connections cannot hold the lock. A holder
// that hangs is a process that never becomes ready, which the orchestrator's
// startup probe restarts, releasing the lock the same way; the waiting
// processes say so every startupLockWaitReport meanwhile, and stop waiting
// when ctx ends.
//
// A connection whose lock may still be held — the release failed — is
// discarded rather than returned to the pool, where it would keep every
// other process out until it was closed.
func lockStartup(ctx context.Context, handler *gorm.DB, purpose string) (unlock func(), err error) {
	noop := func() {}
	if handler.Dialector == nil {
		return noop, nil
	}
	lock, ok := startupLocks[strings.ToLower(handler.Dialector.Name())]
	if !ok {
		return noop, nil
	}
	sqlDB, err := handler.DB()
	if err != nil {
		return noop, err
	}
	if sqlDB.Stats().MaxOpenConnections == 1 {
		return noop, nil
	}
	name := startupLockName(purpose, databaseNameOf(handler))

	connCtx, cancelConn := context.WithTimeout(ctx, startupLockStatementTimeout)
	defer cancelConn()
	conn, err := sqlDB.Conn(connCtx)
	if err != nil {
		return noop, errors.Wrapf(ending(ctx, err), "take a connection for the %s lock", purpose)
	}
	if err := awaitStartupLock(ctx, lock, conn, name, purpose); err != nil {
		_ = conn.Close()
		return noop, errors.Wrapf(err, "take the %s lock", purpose)
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), startupLockStatementTimeout)
		defer cancel()
		if err := lock.release(releaseCtx, conn, name); err != nil {
			zap.S().Warnw("failed to release the startup lock; discarding its connection", "purpose", purpose, "error", err)
			discardConn(conn)
			return
		}
		_ = conn.Close()
	}, nil
}

// awaitStartupLock tries for the lock on conn every startupLockPoll until
// it is taken or ctx ends, and warns every startupLockWaitReport that it is
// still waiting.
func awaitStartupLock(ctx context.Context, lock startupLock, conn *sql.Conn, name, purpose string) error {
	begin := time.Now()
	reported := begin
	for {
		tryCtx, cancel := context.WithTimeout(ctx, startupLockStatementTimeout)
		taken, err := lock.try(tryCtx, conn, name)
		cancel()
		if err != nil {
			return ending(ctx, err)
		}
		if taken {
			return nil
		}
		if time.Since(reported) >= startupLockWaitReport {
			zap.S().Warnw("still waiting for the startup lock held by another process", "purpose", purpose, util.LogDuration(time.Since(begin)))
			reported = time.Now()
		}
		select {
		case <-time.After(startupLockPoll):
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
}

// ending returns why ctx ended when err is that ending's doing — the wait
// was told to stop, and a statement it cut short is not the reason to
// report — and err itself otherwise.
func ending(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return err
}

// databaseNames caches the name of the database behind each handle: the
// tables are prepared one at a time, each under the lock, and the name that
// keys the lock does not change while the process runs, so the server is
// asked once per handle rather than once per table.
var databaseNames sync.Map // *gorm.DB -> string

// databaseNameOf returns the name of the database handler is connected to.
func databaseNameOf(handler *gorm.DB) string {
	if cached, ok := databaseNames.Load(handler); ok {
		if name, ok := cached.(string); ok {
			return name
		}
	}
	name := handler.Migrator().CurrentDatabase()
	databaseNames.Store(handler, name)
	return name
}

// discardConn closes conn's underlying connection instead of returning it to
// the pool: reporting a bad connection from Raw is how database/sql is told
// to drop one.
func discardConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}

// requireTableName returns the model's explicit table name and rejects the
// base default "". gorm's Tabler reads the same TableName method, so a model
// without an explicit name would flow an empty table into every statement;
// failing here turns the missing declaration into a clear startup error
// naming the model instead.
func requireTableName(m types.Model) (string, error) {
	tableName := m.TableName()
	if len(tableName) == 0 {
		return "", errors.Newf("model %T must declare an explicit table name by overriding TableName", m)
	}
	return tableName, nil
}

// tableProgressLogInterval is how often Wait reports the remaining backlog. It
// doubles as the upper bound on how long Wait sleeps in one go, so a lost or
// coalesced wakeup delays the next look at the pending count by this much
// rather than forever.
const tableProgressLogInterval = 500 * time.Millisecond

// Wait blocks until every model registered so far has its table, waking on
// each table that finishes and reporting the remaining backlog on a fixed
// cadence. It reads the pending count rather than the registration queue: a
// model leaves that queue when its preparation starts, not when its table
// exists.
//
// Called before InitDatabase it returns straight away with a warning, because
// nothing is preparing tables yet and there is no end to wait for.
//
// Wait only observes work already queued. If another subsystem, such as module
// registration, can still call model.Register, drain that subsystem first and
// then call Wait so its tables are visible.
func Wait() {
	if tablePreparationStarted.Load() == 0 {
		zap.S().Warnw("Wait() called before InitDatabase(), returning immediately",
			"reason", "processing goroutine not started")
		return
	}

	startTime := time.Now()
	var lastLogTime time.Time

	for {
		pending := modelregistry.TablesPending()
		if pending == 0 {
			break
		}

		// The zero lastLogTime reports the backlog once up front, before the
		// cadence takes over, so a run that stalls on its very first table is
		// visible too.
		if time.Since(lastLogTime) >= tableProgressLogInterval {
			zap.S().Infow(
				"waiting for database initialization",
				util.LogDuration(time.Since(startTime)),
				"total_pending", pending,
			)
			lastLogTime = time.Now()
		}

		awaitTableProgress(lastLogTime)
	}

	elapsed := time.Since(startTime)
	zap.S().Infow(
		"database initialization completed",
		util.LogDuration(elapsed),
	)
}

// awaitTableProgress blocks until a table finishes preparing or the next
// progress report falls due, whichever comes first. Waiting on the signal
// rather than polling is what keeps the last table of a drain from adding a
// poll interval to every startup.
func awaitTableProgress(lastLogTime time.Time) {
	timer := time.NewTimer(time.Until(lastLogTime.Add(tableProgressLogInterval)))
	defer timer.Stop()

	select {
	case <-modelregistry.TablesChanged():
	case <-timer.C:
	}
}
