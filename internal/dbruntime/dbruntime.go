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
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DB holds the framework-managed default GORM database handle.
//
// The database runtime updates it during initialization, and public packages
// expose read-only accessors for application code.
var DB *gorm.DB

// startedTable is an atomic flag to ensure table processing goroutine starts only once
var startedTable atomic.Int32

// initedTable is a concurrent map that tracks initialized tables by their unique key (table_name:db_name)
// It is used by the record processing goroutine to wait for table creation before inserting records
var initedTable = cmap.New[string]()

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
func ensureTable(handler *gorm.DB, m types.Model) error {
	if config.App.Database.AutoMigrate {
		if err := handler.Table(m.GetTableName()).AutoMigrate(m); err != nil {
			return err
		}
		return ensureCustomIndexes(handler, m)
	}
	tableName, err := resolveTableName(handler, m)
	if err != nil {
		return err
	}
	if !handler.Migrator().HasTable(tableName) {
		return errors.Newf("table %q does not exist: run \"gg migrate\" to apply the schema, or enable database.auto_migrate for local development", tableName)
	}
	return nil
}

// InitDatabase initializes database tables and records with asynchronous processing support.
// It creates tables and inserts records that are registered via Register() or RegisterTo() functions.
// The function supports concurrent model registration at any stage - before, during, or after InitDatabase execution.
//
// Key features:
//   - Asynchronous table creation and record insertion using goroutines and channels
//   - Thread-safe concurrent model registration support
//   - Automatic handling of both default database and custom database instances
//   - Real-time processing of models registered during initialization
//
// NOTE: The version of gorm.io/driver/postgres lower than v1.5.4 have some issues.
// More details see: https://github.com/go-gorm/gorm/issues/6886
func InitDatabase(db *gorm.DB, dbmap map[string]*gorm.DB) (err error) {
	// Install GORM OpenTelemetry tracing plugin
	if err = db.Use(otelgorm.NewPlugin()); err != nil {
		zap.S().Warnw("failed to install GORM OpenTelemetry tracing plugin", "error", err)
	}

	// Install tracing plugin for custom databases
	for _, customDB := range dbmap {
		if err = customDB.Use(otelgorm.NewPlugin()); err != nil {
			zap.S().Warnw("failed to install GORM OpenTelemetry tracing plugin for custom DB", "error", err)
		}
	}

	if startedTable.CompareAndSwap(0, 1) {
		go func() {
			for {
				select {
				case m := <-modelregistry.TableChan:
					// Prepare the table in the default database.
					begin := time.Now()

					typ := reflect.TypeOf(m).Elem()
					if err = ensureTable(db, m); err != nil {
						err = errors.Wrap(err, fmt.Sprintf("failed to prepare table(%s)", typ.String()))
						panic(err)
					}
					zap.S().Infow("database table ready", "model", typ.String(), "cost", util.FormatDurationSmart(time.Since(begin)))

					initedTable.Set(typ.String(), "")

				case v := <-modelregistry.TableDBChan:
					if v == nil {
						continue
					}

					// Prepare the table in the custom database.
					begin := time.Now()

					handler := db
					if val, exists := dbmap[strings.ToLower(v.DBName)]; exists {
						handler = val
					}
					m := v.Table
					typ := reflect.TypeOf(m).Elem()
					if err = ensureTable(handler, m); err != nil {
						err = errors.Wrap(err, fmt.Sprintf("failed to prepare table(%s)", typ.String()))
						panic(err)
					}
					zap.S().Infow("database table ready", "model", typ.String(), "cost", util.FormatDurationSmart(time.Since(begin)))

					initedTable.Set(typ.String(), v.DBName)

				case r := <-modelregistry.RecordChan:
					if r == nil {
						continue
					}

					// Create records that must exist before database CRUD operations.
					// NOTE: we should always creates records after table migration finished.
					//
					// We should running this goroutine in a separate goroutine to avoid blocking the main goroutine.
					go func(r *modelregistry.Record) {
						typ := reflect.TypeOf(r.Table).Elem()
						for {
							dbname, e := initedTable.Get(typ.String())
							if e && dbname == r.DBName {
								break
							}
							time.Sleep(100 * time.Millisecond)
						}

						begin := time.Now()
						handler := db
						if val, exists := dbmap[strings.ToLower(r.DBName)]; exists {
							handler = val
						}
						// Use upsert-avoidance to keep seeding idempotent across DBs.
						if err = handler.Table(r.Table.GetTableName()).
							Clauses(clause.OnConflict{DoNothing: true}).
							Create(r.Rows).Error; err != nil {
							err = errors.Wrap(err, "failed to create table records")
							panic(err)
						}
						zap.S().Infow("database create table records", "model", typ.String(), "cost", util.FormatDurationSmart(time.Since(begin)))
					}(r)

				}
			}
		}()
	}

	// set default database to 'Default'.
	DB = db

	return nil
}

// Wait blocks until all pending database initialization operations are completed.
// It monitors three channels used by the InitDatabase function's background goroutine:
//
//   - modelregistry.TableChan: Contains models waiting for table creation in the default database
//   - modelregistry.TableDBChan: Contains models waiting for table creation in custom databases
//   - modelregistry.RecordChan: Contains records waiting for insertion after table creation
//
// This function is useful in scenarios where you need to ensure that all database
// tables and initial records are fully created before proceeding with application logic.
// Common use cases include:
//
//   - Testing environments where you need to wait for complete database setup
//   - Application startup sequences that depend on specific tables being available
//   - Migration scripts that require all tables to be created before data operations
//
// The function polls the channels every 100 milliseconds and prints progress information
// showing the number of pending operations in each channel. It returns only when all
// channels are empty, indicating that the InitDatabase background processing is complete.
//
// NOTE: This function should be called after InitDatabase() has been invoked, as it
// relies on the background goroutine started by InitDatabase to process the channels.
// Calling Wait() before InitDatabase() will return immediately with a warning.
//
// Wait only observes database queues that already contain work. If another
// subsystem, such as module registration, can still call model.Register, drain
// that subsystem first and then call Wait so its tables and records are visible.
func Wait() {
	// Check if InitDatabase has been called and the processing goroutine has started
	if startedTable.Load() == 0 {
		zap.S().Warnw("Wait() called before InitDatabase(), returning immediately",
			"reason", "processing goroutine not started")
		return
	}

	startTime := time.Now()
	var lastLogTime time.Time

	for len(modelregistry.TableChan) != 0 || len(modelregistry.TableDBChan) != 0 || len(modelregistry.RecordChan) != 0 {
		tablePending := len(modelregistry.TableChan)
		tableDBPending := len(modelregistry.TableDBChan)
		recordPending := len(modelregistry.RecordChan)

		// Log progress every 500ms to avoid spam
		if time.Since(lastLogTime) >= 500*time.Millisecond {
			elapsed := time.Since(startTime)
			totalPending := tablePending + tableDBPending + recordPending

			zap.S().Infow(
				"waiting for database initialization",
				"elapsed", util.FormatDurationSmart(elapsed),
				"total_pending", totalPending,
				"default_tables", tablePending,
				"custom_tables", tableDBPending,
				"records", recordPending,
			)
			lastLogTime = time.Now()
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Log completion
	elapsed := time.Since(startTime)
	zap.S().Infow(
		"database initialization completed",
		"total_time", util.FormatDurationSmart(elapsed),
	)
}
