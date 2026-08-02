package dbruntime

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
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

// startedTable is an atomic flag to ensure table processing goroutine starts only once
var startedTable atomic.Int32

// resolveTableName returns the table a model is backed by. Models may either
// declare an explicit name through GetTableName or leave it empty and rely on
// gorm's naming strategy; both forms are supported, so every consumer of a
// model table name must resolve through this helper instead of trusting
// GetTableName alone.
func resolveTableName(handler *gorm.DB, m types.Model) (string, error) {
	if tableName := m.GetTableName(); len(tableName) > 0 {
		return tableName, nil
	}
	stmt := &gorm.Statement{DB: handler}
	if err := stmt.Parse(m); err != nil {
		return "", err
	}
	return stmt.Schema.Table, nil
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
	tableName, err := resolveTableName(handler, m)
	if err != nil {
		return err
	}
	inMemory := config.App.Database.Type == config.DBSqlite && config.App.Sqlite.IsMemory
	if config.App.Database.AutoMigrate || inMemory {
		if err := handler.Table(tableName).AutoMigrate(m); err != nil {
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
				// Prepare the table in the default database.
				begin := time.Now()

				typ := reflect.TypeOf(m).Elem()
				if err = ensureTable(db, m); err != nil {
					err = errors.Wrap(err, fmt.Sprintf("failed to prepare table(%s)", typ.String()))
					panic(err)
				}
				zap.S().Infow("database table ready", "model", typ.String(), util.LogDuration(time.Since(begin)))
			}
		}()
	}

	// set default database to 'Default'.
	DB = db

	return nil
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

	for len(modelregistry.TableChan) != 0 {
		tablePending := len(modelregistry.TableChan)

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
