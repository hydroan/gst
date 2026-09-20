package dbruntime

import (
	"context"
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
		// Recorded rather than panicked: preparation runs on a goroutine of
		// its own, where a panic ends the process where it stands — the
		// buffered log lines never reach the file, the exit code is the
		// runtime's, and the entries already written can read as a start that
		// went fine. Wait hands the failure to the caller, which leaves
		// through the one exit every other startup failure leaves through.
		recordPrepareFailure(errors.Wrapf(err, "failed to prepare table(%s)", typ.String()))
		return
	}
	zap.S().Infow("database table ready", "model", typ.String(), util.LogDuration(time.Since(begin)))
}

// prepareFailure is the first failure the preparation met, the one Wait
// reports. Later ones are not collected: the first one already ends the
// start, and the tables behind it were never going to be prepared.
var prepareFailure struct {
	mu  sync.Mutex
	err error
}

// recordPrepareFailure keeps the first failure and logs every one, so a run
// that met several says so even though only the first ends the start.
func recordPrepareFailure(err error) {
	prepareFailure.mu.Lock()
	defer prepareFailure.mu.Unlock()
	zap.S().Errorw("failed to prepare a database table", "error", err)
	if prepareFailure.err == nil {
		prepareFailure.err = err
	}
}

// preparationFailed returns the failure the preparation met, nil when none
// did.
func preparationFailed() error {
	prepareFailure.mu.Lock()
	defer prepareFailure.mu.Unlock()
	return prepareFailure.err
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
// It returns the first failure the preparation met, if any: preparation runs
// on a goroutine of its own, so this is where a table the deployment could
// not get reaches the caller, in time to end the start through its ordinary
// exit.
//
// Called before InitDatabase it returns straight away with a warning, because
// nothing is preparing tables yet and there is no end to wait for.
//
// Wait only observes work already queued. If another subsystem, such as module
// registration, can still call model.Register, drain that subsystem first and
// then call Wait so its tables are visible.
func Wait() error {
	if tablePreparationStarted.Load() == 0 {
		zap.S().Warnw("Wait() called before InitDatabase(), returning immediately",
			"reason", "processing goroutine not started")
		return nil
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

	if err := preparationFailed(); err != nil {
		return err
	}

	elapsed := time.Since(startTime)
	zap.S().Infow(
		"database initialization completed",
		util.LogDuration(elapsed),
	)
	return nil
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
