package dbruntime

import (
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	prommetrics "github.com/hydroan/gst/metrics"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DB holds the framework-managed default GORM database handle.
//
// The database runtime updates it during initialization, and public packages
// expose read-only accessors for application code.
var DB *gorm.DB

// NowUTC produces every framework-managed timestamp: it is the gorm.Config
// NowFunc shared by the dialect packages (the updated_at refresh on Update,
// the deleted_at stamp on soft delete) and the explicit source
// database.Create/Upsert stamp rows with. Two decisions live here:
//
//   - UTC, the framework's one time base. The gorm default time.Now() carries
//     the server's local zone: drivers with a UTC wire location still store
//     the right instant, but the in-memory model then serializes with a local
//     offset instead of the UTC form read rows carry, and sqlite would even
//     persist that local offset into the row text.
//   - Millisecond truncation, matching the millisecond storage precision the
//     dialects share (MySQL datetime(3), ClickHouse DateTime64(3)). Without
//     it the in-memory value keeps nanoseconds the row cannot hold, so the
//     timestamp a write hands back differs from what a later read returns —
//     and MySQL rounds half up, which can even shift the stored millisecond.
func NowUTC() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }

// startedTable is an atomic flag to ensure table processing goroutine starts only once
var startedTable atomic.Int32

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
		// AutoMigrate reads the table name through gorm's Tabler, which is
		// the model's own TableName method. Supplying it again through
		// Table() would make gorm re-parse the schema under a special table
		// name, which renames the constraints of associated models.
		if err := handler.AutoMigrate(m); err != nil {
			return err
		}
		return ensureCustomIndexes(handler, m)
	}
	if !handler.Migrator().HasTable(tableName) {
		return errors.Newf("table %q does not exist: run \"gg migrate\" to apply the schema, or enable database.auto_migrate for local development", tableName)
	}
	return nil
}

// InitDatabase initializes database tables with asynchronous processing support.
// It creates tables for models registered via the model.Register function.
// The function supports concurrent model registration at any stage - before, during, or after InitDatabase execution.
//
// Key features:
//   - Asynchronous table creation using a goroutine and channel
//   - Thread-safe concurrent model registration support
//   - Real-time processing of models registered during initialization
//
// NOTE: The version of gorm.io/driver/postgres lower than v1.5.4 have some issues.
// More details see: https://github.com/go-gorm/gorm/issues/6886
func InitDatabase(db *gorm.DB) (err error) {
	if startedTable.CompareAndSwap(0, 1) {
		go func() {
			for m := range modelregistry.TableChan {
				prepareTable(db, m)
			}
		}()
	}

	// set default database to 'Default'.
	DB = db

	registerPoolMetrics(db)
	return nil
}

// registerPoolMetrics exposes the default database's connection pools to the
// metrics registry: the single pool of a plain handle under the stable name
// "default", and with replicas attached, every node — the primary as
// "default" and replicas as "default_replica_N". Failures only log:
// observability must never block startup, and a deployment without a metrics
// endpoint simply leaves the collectors unserved.
func registerPoolMetrics(db *gorm.DB) {
	if nodes := NodesFor(db); len(nodes) > 0 {
		names := ReplicaPoolMetricNames("default", nodes)
		for i, node := range nodes {
			if err := prommetrics.RegisterDBStats(node.DB, names[i]); err != nil {
				zap.S().Warnw("failed to register database pool metrics collector", "db_name", names[i], "error", err)
			}
		}
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		zap.S().Warnw("failed to reach sql.DB for pool metrics", "error", err)
		return
	}
	if err := prommetrics.RegisterDBStats(sqlDB, "default"); err != nil {
		zap.S().Warnw("failed to register database pool metrics collector", "db_name", "default", "error", err)
	}
}

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

// Wait blocks until all pending database initialization operations are completed.
// It monitors the table channel used by the InitDatabase function's background
// goroutine: modelregistry.TableChan contains models waiting for table creation
// in the default database.
//
// This function is useful in scenarios where you need to ensure that all database
// tables are fully created before proceeding with application logic.
// Common use cases include:
//
//   - Testing environments where you need to wait for complete database setup
//   - Application startup sequences that depend on specific tables being available
//   - Migration scripts that require all tables to be created before data operations
//
// The function polls the channel every 100 milliseconds and prints progress
// information showing the number of pending operations. It returns only when the
// channel is empty, indicating that the InitDatabase background processing is complete.
//
// NOTE: This function should be called after InitDatabase() has been invoked, as it
// relies on the background goroutine started by InitDatabase to process the channel.
// Calling Wait() before InitDatabase() will return immediately with a warning.
//
// Wait only observes database queues that already contain work. If another
// subsystem, such as module registration, can still call model.Register, drain
// that subsystem first and then call Wait so its tables are visible.
func Wait() {
	// Check if InitDatabase has been called and the processing goroutine has started
	if startedTable.Load() == 0 {
		zap.S().Warnw("Wait() called before InitDatabase(), returning immediately",
			"reason", "processing goroutine not started")
		return
	}

	startTime := time.Now()
	var lastLogTime time.Time

	for modelregistry.TablesPending() != 0 {
		tablePending := modelregistry.TablesPending()

		// Log progress every 500ms to avoid spam
		if time.Since(lastLogTime) >= 500*time.Millisecond {
			elapsed := time.Since(startTime)

			zap.S().Infow(
				"waiting for database initialization",
				util.LogDuration(elapsed),
				"total_pending", tablePending,
			)
			lastLogTime = time.Now()
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Log completion
	elapsed := time.Since(startTime)
	zap.S().Infow(
		"database initialization completed",
		util.LogDuration(elapsed),
	)
}
