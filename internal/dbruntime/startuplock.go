package dbruntime

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// This file holds the startup lock: the advisory lock on the primary database
// that makes the processes of a deployment take a startup step one at a time,
// so that table preparation and seeding do not race each other across
// replicas.

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
// session, and a holder that crashes drops it with its connection — which is
// also why the process must reach the primary directly or through a
// session-level pool, since a proxy that hands a session's statements to
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
		// The lock lives on a connection of its own, held for as long as the
		// step runs: a pool of one has none to spare, and the step would run
		// on every replica at once with nothing saying so. A dialect that
		// offers no lock at all is answered above — this one offers it, and
		// the deployment cannot use it, which the capability-miss rule makes
		// a startup failure rather than a silent difference.
		return noop, errors.Errorf(
			"the %s lock needs a connection of its own: raise database.max_open_conns to at least 2", purpose)
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
// report — and err itself otherwise. It goes by ctx, not by err: a driver
// reports a statement cut short in words of its own, not as ctx's error, so
// a failure of the statement's own that coincides with the stop is reported
// as the stop; the process is ending either way.
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
