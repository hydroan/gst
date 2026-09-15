package dbruntime

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"hash/fnv"
	"reflect"
	"strings"
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

// migrateTable runs gorm AutoMigrate and creates the custom indexes, once
// across the processes sharing the database. Replicas starting together all
// find the table missing and all issue CREATE TABLE, and the server refuses
// every one but the first: on MySQL and PostgreSQL the processes take turns
// under a server-side advisory lock, and where none can be held — SQLite,
// or a pool of a single connection — a failure while another process created
// the table is retried once, against the table that is there now.
//
// AutoMigrate reads the table name through gorm's Tabler, which is the
// model's own TableName method. Supplying it again through Table() would make
// gorm re-parse the schema under a special table name, which renames the
// constraints of associated models.
func migrateTable(handler *gorm.DB, m types.Model, tableName string) error {
	unlock, err := lockMigration(handler)
	if err != nil {
		return err
	}
	defer unlock()

	migrate := func() error {
		if migrateErr := handler.AutoMigrate(m); migrateErr != nil {
			return migrateErr
		}
		return ensureCustomIndexes(handler, m)
	}
	err = migrate()
	if err != nil && handler.Migrator().HasTable(tableName) {
		err = migrate()
	}
	return err
}

// migrationLockName is the advisory lock table preparation holds on the
// servers that offer one: a single name for the whole schema, so that the
// processes of a deployment prepare their tables one process at a time.
const migrationLockName = "gst:migrate"

// migrationLockKey is the same lock as an integer, for the server that keys
// advisory locks by one: a stable hash of the name.
var migrationLockKey = func() int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(migrationLockName))
	return int64(h.Sum32())
}()

// migrationLockTimeout bounds the wait for the lock. A holder is creating a
// table, which takes milliseconds; a minute covers a server that is slow to
// come up under a whole deployment starting at once.
const migrationLockTimeout = time.Minute

// migrationLock is the advisory lock of one dialect, taken and released on
// the connection that holds it.
type migrationLock struct {
	acquire func(ctx context.Context, conn *sql.Conn) error
	release func(ctx context.Context, conn *sql.Conn) error
}

// migrationLocks are the advisory locks table preparation holds, by dialect.
// MySQL names its locks and answers the wait itself: 1 for the lock taken, 0
// for the timeout and NULL for an error. PostgreSQL keys them by an integer
// and blocks until the lock is taken, so the wait is bounded by the context.
var migrationLocks = map[string]migrationLock{
	"mysql": {
		acquire: func(ctx context.Context, conn *sql.Conn) error {
			var got sql.NullInt64
			if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", migrationLockName, int64(migrationLockTimeout.Seconds())).Scan(&got); err != nil {
				return err
			}
			if !got.Valid || got.Int64 != 1 {
				return errors.Newf("not granted within %s: another process is still preparing tables", migrationLockTimeout)
			}
			return nil
		},
		release: func(ctx context.Context, conn *sql.Conn) error {
			_, err := conn.ExecContext(ctx, "DO RELEASE_LOCK(?)", migrationLockName)
			return err
		},
	},
	"postgres": {
		acquire: func(ctx context.Context, conn *sql.Conn) error {
			_, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey)
			return err
		},
		release: func(ctx context.Context, conn *sql.Conn) error {
			_, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockKey)
			return err
		},
	},
}

// lockMigration takes the advisory lock table preparation runs under and
// returns the function that releases it. Both servers scope the lock to the
// session, so it is held on a connection of its own while the migration runs
// on the pool's other connections. A pool of a single connection cannot hold
// the lock and migrate at once, and SQLite has no such lock: there nothing is
// locked, and the retries of migrateTable and ensureCustomIndexes are what
// cover a race.
//
// A connection whose lock may still be held — the release failed, or the
// wait was cut short — is discarded rather than returned to the pool, where
// it would keep every other process out until it was closed.
func lockMigration(handler *gorm.DB) (unlock func(), err error) {
	noop := func() {}
	lock, ok := migrationLocks[strings.ToLower(handler.Dialector.Name())]
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

	// MySQL times the wait out itself; the margin lets its verdict arrive
	// before the context's.
	ctx, cancel := context.WithTimeout(context.Background(), migrationLockTimeout+5*time.Second)
	defer cancel()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return noop, errors.Wrap(err, "take a connection for the migration lock")
	}
	if err := lock.acquire(ctx, conn); err != nil {
		discardConn(conn)
		return noop, errors.Wrap(err, "take the migration lock")
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), migrationLockTimeout)
		defer cancel()
		if err := lock.release(ctx, conn); err != nil {
			zap.S().Warnw("failed to release the migration lock; discarding its connection", "error", err)
			discardConn(conn)
			return
		}
		_ = conn.Close()
	}, nil
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
