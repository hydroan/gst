package database

import (
	"reflect"
	"time"

	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Transaction executes a function within a database transaction with automatic context injection.
// This is the RECOMMENDED method for single-model transactions as it automatically provides
// a transaction-aware database instance, preventing the common mistake of forgetting WithTx(tx).
//
// The method provides:
// 1. Automatic transaction begin/commit/rollback management
// 2. Automatic transaction context injection (no need to call WithTx manually)
// 3. Built-in error handling and logging
// 4. Performance monitoring with execution time tracking
// 5. Type-safe transaction context
//
// Transaction behavior:
// - If the function returns nil: transaction is automatically committed
// - If the function returns an error: transaction is automatically rolled back
// - If the function panics: the underlying GORM transaction is rolled back before the panic propagates
// - Transaction locks acquired by tx are released by commit or rollback, including panic rollback
// - Custom rollback function (set via WithRollback) is executed only when the function returns an error
// - All operations through tx are automatically within the transaction
//
// Use cases:
// - Use Transaction: For single-model transactions (recommended, safer)
// - Use TransactionFunc: For multi-model transactions (requires manual WithTx calls)
//
// Example - Simple transaction:
//
//	err := database.Database[*model.User](context.Background()).Transaction(func(tx types.Database[*model.User]) error {
//	    // tx already has transaction context - no need for WithTx!
//	    if err := tx.Create(&user); err != nil {
//	        return err // Automatic rollback
//	    }
//	    return tx.UpdateByID(user.ID, "status", "active") // Automatic commit
//	})
//
// Example - Complex transaction with query options:
//
//	err := database.Database[*model.Order](context.Background()).Transaction(func(tx types.Database[*model.Order]) error {
//	    // All query options work as expected
//	    if err := tx.WithLock(consts.LockUpdate).Get(&order, orderID); err != nil {
//	        return err
//	    }
//	    order.Status = "processed"
//	    return tx.Update(&order)
//	})
//
// Example - With custom rollback:
//
//	err := database.Database[*model.User](context.Background()).WithRollback(func() {
//	    // Custom cleanup logic
//	}).Transaction(func(tx types.Database[*model.User]) error {
//	    return tx.Create(&user)
//	})
//
// For multi-model transactions, use TransactionFunc instead.
//
// Returns ErrNilTransactionFunc if fn is nil.
func (db *database[M]) Transaction(fn func(tx types.Database[M]) error) error {
	defer db.reset()

	if fn == nil {
		return ErrNilTransactionFunc
	}
	if err := db.prepare(); err != nil {
		return err
	}
	if db.buildingSQL {
		return ErrBuildSQLTransaction
	}

	modelName := reflect.TypeOf(*new(M)).Elem().Name()
	spanCtx, span := gstotel.StartSpan(db.ctx, transactionSpanName(modelName, "Transaction"))
	defer span.End()
	gstotel.AddSpanTags(span, map[string]any{
		"component":          "database",
		"database.model":     modelName,
		"database.operation": "Transaction",
	})

	begin := time.Now()

	// Binding the transaction handle to spanCtx makes GORM's Begin copy the span context
	// into gormTx's statement context, which WithTx lifts back out to nest inner operation
	// spans under this transaction span.
	txErr := db.ins.WithContext(spanCtx).Transaction(func(gormTx *gorm.DB) error {
		// Create a new database instance with transaction context. Using spanCtx (rather
		// than db.ctx) makes per-statement spans from GormTracingPlugin nest under this
		// transaction span instead of appearing as flat siblings in the trace.
		tx := Database[M](spanCtx).WithTx(gormTx)

		// Copy relevant options to the transaction database instance
		if db.rollbackFunc != nil {
			tx = tx.WithRollback(db.rollbackFunc)
		}

		// Execute the user function with the transaction database instance
		if err := fn(tx); err != nil {
			// Execute custom rollback logic if provided
			if db.rollbackFunc != nil {
				db.rollbackFunc()
			}
			logger.Database.WithContext(db.ctx, consts.Phase("Transaction")).Errorz(
				"transaction rolled back due to error",
				zap.Error(err),
				zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
			)
			return err
		}

		logger.Database.WithContext(db.ctx, consts.Phase("Transaction")).Infoz(
			"transaction committed successfully",
			zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
		)
		return nil
	})

	// Recorded after db.ins.Transaction returns so failures during the commit phase itself
	// are also captured on the span. The span's own native duration already covers the full
	// transaction including commit/rollback, so no duration tag is added.
	gstotel.RecordError(span, txErr)
	return txErr
}

// TransactionFunc executes a function within a complete transaction with automatic management.
// This method is designed for MULTI-MODEL transactions where you need to operate on different
// model types within the same transaction. For single-model transactions, use Transaction instead.
//
// IMPORTANT: You MUST manually call WithTx(tx) for each database instance to ensure operations
// are part of the transaction. Forgetting WithTx will cause operations to execute outside the
// transaction and will NOT be rolled back on error.
//
// The method provides:
// 1. Automatic transaction begin/commit/rollback management
// 2. Built-in error handling and logging
// 3. Performance monitoring with execution time tracking
// 4. Support for multiple model types in the same transaction
//
// Transaction behavior:
// - If the function returns nil: transaction is automatically committed
// - If the function returns an error: transaction is automatically rolled back
// - If the function panics: the underlying GORM transaction is rolled back before the panic propagates
// - Transaction locks acquired by operations using WithTx(tx) are released by commit or rollback, including panic rollback
// - Operations that call WithTx(tx) are executed in the same transaction
// - Operations that do not call WithTx(tx) execute outside this transaction and are not affected by its rollback
// - Custom rollback function (set via WithRollback) is executed only when the function returns an error
//
// Relationship with other transaction methods:
// - Use Transaction: For single-model transactions (recommended, safer - auto WithTx)
// - Use TransactionFunc: For multi-model transactions (requires manual WithTx)
// - Use WithRollback: To add custom cleanup logic on transaction failure
//
// Example - Multi-model transaction:
//
//	err := database.Database[*model.User](context.Background()).TransactionFunc(func(tx any) error {
//	    userDB := database.Database[*model.User](context.Background()).WithTx(tx)    // Must use WithTx!
//	    orderDB := database.Database[*model.Order](context.Background()).WithTx(tx)  // Must use WithTx!
//
//	    if err := userDB.Create(&user); err != nil {
//	        return err // Automatic rollback
//	    }
//	    if err := orderDB.Create(&order); err != nil {
//	        return err // Automatic rollback
//	    }
//	    return nil // Automatic commit
//	})
//
// Example - Complex multi-model transaction:
//
//	err := database.Database[*model.Order](context.Background()).TransactionFunc(func(tx any) error {
//	    orderDB := database.Database[*model.Order](context.Background()).WithTx(tx)
//	    userDB := database.Database[*model.User](context.Background()).WithTx(tx)
//
//	    // Get and lock order
//	    if err := orderDB.WithLock(consts.LockUpdate).Get(&order, orderID); err != nil {
//	        return err
//	    }
//	    // Update user points
//	    if err := userDB.UpdateByID(order.UserID, "points", user.Points+100); err != nil {
//	        return err
//	    }
//	    // Update order status
//	    order.Status = "processed"
//	    return orderDB.Update(&order)
//	})
//
// Example - With custom rollback:
//
//	err := db.WithRollback(func() {
//	    // Custom rollback logic
//	}).TransactionFunc(func(tx any) error {
//	    userDB := database.Database[*model.User](context.Background())
//	    if err := userDB.WithTx(tx).Create(&user); err != nil {
//	        return err // Automatic rollback, rollback function will be called
//	    }
//	    return nil // Automatic commit, rollback function will NOT be called
//	})
//
// Returns ErrNilTransactionFunc if fn is nil.
func (db *database[M]) TransactionFunc(fn func(tx any) error) error {
	defer db.reset()

	if fn == nil {
		return ErrNilTransactionFunc
	}
	if err := db.prepare(); err != nil {
		return err
	}
	if db.buildingSQL {
		return ErrBuildSQLTransaction
	}

	modelName := reflect.TypeOf(*new(M)).Elem().Name()
	spanCtx, span := gstotel.StartSpan(db.ctx, transactionSpanName(modelName, "TransactionFunc"))
	defer span.End()
	gstotel.AddSpanTags(span, map[string]any{
		"component":          "database",
		"database.model":     modelName,
		"database.operation": "TransactionFunc",
	})

	begin := time.Now()

	// fn only receives the raw *gorm.DB, so the span context travels inside the transaction
	// handle's statement context (GORM's Begin copies it from this handle): WithTx(tx) lifts
	// it back out to nest the spans of every operation that joins this transaction.
	txErr := db.ins.WithContext(spanCtx).Transaction(func(tx *gorm.DB) error {
		// Execute the user function with the transaction gorm.DB instance
		if err := fn(tx); err != nil {
			// Execute custom rollback logic if provided
			if db.rollbackFunc != nil {
				db.rollbackFunc()
			}
			logger.Database.WithContext(db.ctx, consts.Phase("TransactionFunc")).Errorz(
				"transaction rolled back due to error",
				zap.Error(err),
				zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
			)
			return err
		}

		logger.Database.WithContext(db.ctx, consts.Phase("TransactionFunc")).Infoz(
			"transaction committed successfully",
			zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
		)
		return nil
	})

	// Recorded after db.ins.Transaction returns so failures during the commit phase itself
	// are also captured on the span. The span's own native duration already covers the full
	// transaction including commit/rollback, so no duration tag is added.
	gstotel.RecordError(span, txErr)
	return txErr
}
