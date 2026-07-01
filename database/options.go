package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"gorm.io/gorm"
)

// WithDB sets the underlying GORM database instance for this database manipulator.
// This allows switching between different database connections or configurations.
// The custom database must already contain the tables required by the operation.
//
// Parameters:
//   - x: The GORM database instance (*gorm.DB). If nil or invalid, returns the original instance.
//
// Behavior:
//   - Switches the operation chain to the provided database instance
//   - Preserves context from the original database instance
//   - Uses the provided database as-is
//
// Examples:
//
//	// Use custom database instance
//	customDB := sqlite.New(config.Sqlite{...})
//	database.Database[*model.User](context.Background()).WithDB(customDB).Create(&user)
//
//	// Combined with WithTable
//	database.Database[*model.User](context.Background()).WithDB(customDB).WithTable("users").List(&users)
//
//	// Multiple database instances
//	db1 := sqlite.New(config.Sqlite{Path: "/tmp/db1.db"})
//	db2 := sqlite.New(config.Sqlite{Path: "/tmp/db2.db"})
//	database.Database[*model.User](context.Background()).WithDB(db1).Create(&user1)
//	database.Database[*model.User](context.Background()).WithDB(db2).Create(&user2)
//
// NOTE: WithDB expects the required tables to already exist in the target database.
// NOTE: Invalid database type (not *gorm.DB) will log a warning and return the original instance.
func (db *database[M]) WithDB(x any) types.Database[M] {
	var empty *gorm.DB
	if x == nil || x == new(gorm.DB) || x == empty {
		return db
	}
	// v := reflect.ValueOf(x)
	// if v.Kind() != reflect.Pointer {
	// 	return db
	// }
	// if v.IsNil() {
	// 	return db
	// }
	_db, ok := x.(*gorm.DB)
	if !ok {
		logger.Database.WithContext(db.ctx, consts.Phase("WithDB")).Warn("invalid database type, expect *gorm.DB")
		return db
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	ctx := db.ins.Statement.Context
	if ctx == nil {
		ctx = context.Background()
		if db.ctx != nil {
			ctx = db.ctx
		}
	}
	// Keep existing setup bookkeeping for compatibility with prior operation-chain state.
	if db.shouldAutoMigrate == nil {
		// Use database identifier + model type as key to support multiple database instances
		dbIdentifier := getDBIdentifier(_db)
		modelType := reflect.TypeFor[M]().String()
		migrationKey := fmt.Sprintf("%s:%s", dbIdentifier, modelType)
		if _, loaded := migratedModelMap.LoadOrStore(migrationKey, struct{}{}); !loaded {
			flag := new(bool)
			*flag = true
			db.shouldAutoMigrate = flag
		}
	}
	if strings.ToLower(config.App.Logger.Level) == "debug" {
		db.ins = _db.WithContext(ctx).Debug().Limit(defaultLimit)
	} else {
		db.ins = _db.WithContext(ctx).Limit(defaultLimit)
	}
	return db
}

// WithTable sets the table name for database operations, overriding the default table name
// derived from the model type. This is useful for working with custom table names or views.
//
// Parameters:
//   - name: The custom table name to use. Overrides the model's GetTableName() result.
//
// Behavior:
//   - Overrides the default table name for all subsequent operations
//   - Often used in combination with WithDB to work with custom databases and tables
//   - Uses the named table as-is
//
// Examples:
//
//	// Use custom table name
//	database.Database[*model.User](context.Background()).WithTable("custom_users").List(&users)
//
//	// Combined with WithDB
//	customDB := sqlite.New(config.Sqlite{...})
//	// Assume the "users" table already exists in customDB.
//	database.Database[*model.User](context.Background()).WithDB(customDB).WithTable("users").Create(&user)
//
//	// Chainable with other methods
//	database.Database[*model.User](context.Background()).
//	    WithDB(customDB).
//	    WithTable("users").
//	    WithQuery(&model.User{Name: "John"}).
//	    List(&users)
//
// NOTE: The table must exist in the database before performing operations.
func (db *database[M]) WithTable(name string) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.shouldAutoMigrate = new(bool)
	db.tableName = name
	return db
}

// WithTx returns a new database manipulator with transaction context.
// This method allows using an existing transaction to operate on multiple resource types.
// The tx parameter should be a *gorm.DB transaction instance obtained from TransactionFunc.
//
// Parameters:
//   - tx: The transaction instance (*gorm.DB) from TransactionFunc callback.
//     If nil or invalid, logs a warning and returns the original database instance.
//
// Supports all CRUD operations and can be chained with other methods.
//
// Examples:
//
//	// Single resource type transaction
//	database.Database[*User](context.Background()).TransactionFunc(func(tx any) error {
//	    return database.Database[*User](context.Background()).WithTx(tx).Create(&user)
//	})
//
//	// Multiple resource types in the same transaction
//	database.Database[*User](context.Background()).TransactionFunc(func(tx any) error {
//	    if err := database.Database[*User](context.Background()).WithTx(tx).Create(&user); err != nil {
//	        return err
//	    }
//	    if err := database.Database[*Order](context.Background()).WithTx(tx).Create(&order); err != nil {
//	        return err
//	    }
//	    return nil
//	})
//
//	// Chainable with other methods
//	database.Database[*User](context.Background()).TransactionFunc(func(tx any) error {
//	    return database.Database[*User](context.Background()).
//	        WithTx(tx).
//	        WithQuery(&User{Name: "John"}).
//	        Update(&user)
//	})
//
// NOTE: WithTx must be used within a TransactionFunc callback. The transaction is automatically
//
//	committed when the callback returns nil, or rolled back when it returns an error.
//
// WithTx also stores the transaction in this operation chain's context. That
// context propagation matters for model hooks: if a hook receives this context
// and calls Database[*OtherModel](ctx), the new operation chain inherits the
// same transaction instead of opening a separate write.
//
// NOTE: Invalid tx parameter (nil or wrong type) will log a warning and skip transaction context.
func (db *database[M]) WithTx(tx any) types.Database[M] {
	var empty *gorm.DB
	if tx == nil || tx == new(gorm.DB) || tx == empty {
		logger.Database.WithContext(db.ctx, consts.Phase("WithTx")).Warn("invalid database type, expect *gorm.DB")
		return db
	}

	_tx, ok := tx.(*gorm.DB)
	if !ok || _tx == nil {
		logger.Database.WithContext(db.ctx, consts.Phase("WithTx")).Warn("invalid database type, expect *gorm.DB")
		return db
	}

	// return &database[M]{
	// 	ins:          _tx.Model(*new(M)),
	// 	ctx:          db.ctx,
	// 	rollbackFunc: db.rollbackFunc,
	// }
	db.ctx = contextWithTx(db.ctx, _tx)
	db.ins = _tx.Session(&gorm.Session{
		SkipDefaultTransaction: false,
		NewDB:                  false,
	}).WithContext(db.ctx)

	return db
}

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
//	    WithDebug().
//	    Create(users...)
//
// NOTE: If size is 0 or not set, default batch sizes are used (1000 for Create/Update, 10000 for Delete).
// NOTE: The batch size setting applies only to the current operation chain and is reset afterward.
func (db *database[M]) WithBatchSize(size int) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	// db.db = db.db.Session(&gorm.Session{CreateBatchSize: db.batchSize})
	db.batchSize = size
	return db
}

// WithDebug enables debug mode for database operations, showing detailed SQL queries and execution info.
// This setting has higher priority than config.App.Logger.Level and overrides the default value (false).
func (db *database[M]) WithDebug() types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.ins = db.ins.Debug()
	return db
}

// WithRollback sets a rollback callback function that will be executed when a transaction fails.
// This method works with both Transaction and TransactionFunc to enable custom rollback logic
// (e.g., cleaning up external resources, sending notifications).
// The rollback function is only called when the transaction fails (returns an error).
// The rollback function does not return an error - any errors should be handled internally (e.g., logged).
//
// Example with Transaction:
//
//	err := db.WithRollback(func() {
//	    // Custom rollback logic (e.g., cleanup external resources, send notifications)
//	    // This function is called automatically when transaction fails
//	}).Transaction(func(tx types.Database[*model.User]) error {
//	    if err := tx.Create(&user); err != nil {
//	        return err // automatic rollback, rollback function will be called
//	    }
//	    return nil // automatic commit, rollback function will NOT be called
//	})
//
// Example with TransactionFunc:
//
//	err := db.WithRollback(func() {
//	    // Custom rollback logic
//	}).TransactionFunc(func(tx any) error {
//	    userDB := database.Database[*model.User](context.Background())
//	    if err := userDB.WithTx(tx).Create(&user); err != nil {
//	        return err // automatic rollback, rollback function will be called
//	    }
//	    return nil // automatic commit, rollback function will NOT be called
//	})
func (db *database[M]) WithRollback(rollbackFunc func()) types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.rollbackFunc = rollbackFunc
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
//   - Cache entries are not read, cleared, deleted, or written
//   - Input model objects are left unchanged; no ID, timestamp, or soft-delete fields are filled
//
// Example:
//
//	WithDryRun().Create(&user)              // Build INSERT SQL without creating record
//	WithDryRun().Update(&user)              // Build UPDATE SQL without updating record
//	WithDryRun().Delete(&user)              // Build DELETE SQL without deleting record
//	WithDryRun().UpdateByID(id, "name", v)  // Build UPDATE SQL without updating record
//	WithDryRun().List(&users)               // Build SELECT SQL without loading records
//	WithDryRun().Cleanup()                  // Build cleanup DELETE SQL without removing rows
//
// WithDryRun is build-only: it does not execute generated SQL, model hooks, cache mutation, or object field filling.
func (db *database[M]) WithDryRun() types.Database[M] {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.dryRun = true
	return db
}

// WithBuildSQL enables SQL build mode for the next terminal operation.
// It appends generated Query, Args, and RenderedSQL values to statements without
// executing database I/O, model hooks, cache mutation, or object field filling.
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
