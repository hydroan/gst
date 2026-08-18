package database

import (
	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

// This file holds the execution-mode options: how an operation runs (batch
// size, dry run, SQL capture, hook suppression) rather than what it queries.
// The query-shaping options — conditions, ordering, paging, locking and
// friends — live in query_options.go and with_query.go.

// WithBatchSize sets the batch size for batch operations such as batch insert, update, or delete.
// Controls how many records are processed in a single database operation to optimize performance.
//
// Parameters:
//   - size: The number of records to process per batch.
//     If set to 0 or negative, uses default batch sizes:
//   - Create/Update: 1000 records per batch
//   - Delete: 10000 records per batch
//     If set to a positive value, uses that value for all operations.
//
// Affected Operations:
//   - Create: Batch inserts records in chunks of the specified size
//   - Update: Batch updates records in chunks of the specified size
//   - Delete: Batch deletes records in chunks of the specified size
//     Note: Delete operations use a separate default (10000) if size is not set
//
// Performance Considerations:
//   - Larger batch sizes improve performance by reducing database round trips
//   - However, larger batches consume more memory and may hit database limits
//   - Recommended range: 100-5000 for most use cases
//   - Very large batches (>10000) may cause memory issues or exceed database limits
//
// Examples:
//
//	// Set batch size for Create operation
//	database.Database[*model.User](context.Background()).WithBatchSize(1000).Create(users...)
//
//	// Set batch size for Update operation
//	database.Database[*model.User](context.Background()).WithBatchSize(500).Update(users...)
//
//	// Set batch size for Delete operation
//	database.Database[*model.User](context.Background()).WithBatchSize(2000).Delete(users...)
//
//	// Combined with other methods
//	database.Database[*model.User](context.Background()).
//	    WithBatchSize(1000).
//	    Create(users...)
//
// NOTE: If size is 0 or not set, default batch sizes are used (1000 for Create/Update, 10000 for Delete).
// NOTE: The batch size setting applies only to the current operation chain and is reset afterward.
func (db *database[M]) WithBatchSize(size int) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.batchSize = size
	return db
}

// WithDryRun enables dry-run mode to build SQL without executing database I/O.
// Useful for debugging, query optimization, testing query generation, and measuring
// framework overhead without touching the backing database.
// The generated SQL will be built by GORM but not executed against the database.
//
// Behavior:
//   - Create/Update/Delete/UpdateByID: Builds SQL without modifying database rows
//   - List/Get/Count/First/Last/Take: Builds SQL without reading database rows
//   - Cleanup: Builds cleanup DELETE SQL without permanently removing soft-deleted rows
//   - Health: Not affected; it still executes connection checks
//   - Read operations leave destination values unchanged because no rows are loaded
//   - Model hooks are not executed because dry-run is limited to SQL construction
//   - Input model objects are left unchanged; no ID, timestamp, or soft-delete fields are filled
//
// Example:
//
//	WithDryRun().Create(&user)              // Build INSERT SQL without creating record
//	WithDryRun().Update(&user)              // Build UPDATE SQL without updating record
//	WithDryRun().Delete(&user)              // Build DELETE SQL without deleting record
//	WithDryRun().UpdateByID(id, SampleCols.Name.Set(v))  // Build UPDATE SQL without updating record
//	WithDryRun().List(&users)               // Build SELECT SQL without loading records
//	WithDryRun().Cleanup()                  // Build cleanup DELETE SQL without removing rows
//
// WithDryRun is build-only: it does not execute generated SQL, model hooks, or object field filling.
func (db *database[M]) WithDryRun() types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.dryRun = true
	return db
}

// dryRunSession derives the session a dry run builds its statement on.
//
// Statement logging is off because a dry run never reaches the database. The
// SQL log reports what a statement did — how long it took, how many rows it
// touched, which caller issued it — and a statement that was only built has
// none of that to report. Logging it anyway files an entry saying a write
// happened that never did, into the record an operator reconstructs history
// from. Routing every dry run through here keeps that decision in one place
// instead of leaving each operation to remember it.
//
// The default transaction is skipped for the same reason: gorm's
// BeginTransaction callback consults SkipDefaultTransaction only, not DryRun,
// so without the skip a dry-run write still opens a real BEGIN/COMMIT pair on
// the connection pool — database I/O issued for a statement that never runs.
func dryRunSession(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{DryRun: true, SkipDefaultTransaction: true, Logger: glogger.Default.LogMode(glogger.Silent)})
}

// WithBuildSQL enables SQL build mode for the next terminal operation.
// It appends generated Query, Args, and RenderedSQL values to statements without
// executing database I/O, model hooks, or object field filling.
//
// WithBuildSQL is intended for CRUD, read, cleanup, and health-check SQL generation.
// Transaction helpers are not supported because they manage real transaction control flow.
//
// Example:
//
//	var statements []types.SQLStatement
//	err := database.Database[*User](context.Background()).
//	    WithBuildSQL(&statements).
//	    WithQuery(&User{Name: "John"}).
//	    List(&users)
func (db *database[M]) WithBuildSQL(statements *[]types.SQLStatement) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.dryRun = true
	db.buildingSQL = true
	db.sqlStatements = statements
	return db
}

// collectSQL appends generated SQL to the active WithBuildSQL collector.
// It preserves placeholders in Query, keeps bound values in Args, and stores
// dialect-rendered SQL in RenderedSQL for inspection.
func (db *database[M]) collectSQL(tx *gorm.DB) error {
	if tx == nil {
		return nil
	}
	if !db.buildingSQL {
		return tx.Error
	}
	if db.sqlStatements == nil {
		return ErrNilSQLBuilder
	}
	if tx.Statement != nil {
		if query := tx.Statement.SQL.String(); len(query) > 0 {
			args := append([]any(nil), tx.Statement.Vars...)
			renderedSQL := query
			if tx.Dialector != nil {
				renderedSQL = tx.Dialector.Explain(query, args...)
			}
			db.mu.Lock()
			*db.sqlStatements = append(*db.sqlStatements, types.SQLStatement{
				Query:       query,
				Args:        args,
				RenderedSQL: renderedSQL,
			})
			db.mu.Unlock()
		}
	}
	return tx.Error
}

// WithoutHook disables model hooks (callbacks) for the current operation.
// Bypasses BeforeCreate, AfterCreate, BeforeUpdate, AfterUpdate, etc. hooks.
// Use when you need direct database operations without business logic interference.
//
// Example:
//
//	WithoutHook().Create(&user)  // Create without triggering hooks
//	WithoutHook().Update(&user)  // Update without validation hooks
//
// WithoutHook will disable all model hooks.
func (db *database[M]) WithoutHook() types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.noHook = true
	return db
}
