package database

import (
	"time"

	"github.com/hydroan/gst/logger"
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
// - Custom rollback function (set via WithRollback) is executed only when transaction fails
// - All operations through txDB are automatically within the transaction
//
// Use cases:
// - Use Transaction: For single-model transactions (recommended, safer)
// - Use TransactionFunc: For multi-model transactions (requires manual WithTx calls)
//
// Example - Simple transaction:
//
//	err := database.Database[*model.User](nil).Transaction(func(txDB types.Database[*model.User]) error {
//	    // txDB already has transaction context - no need for WithTx!
//	    if err := txDB.Create(&user); err != nil {
//	        return err // Automatic rollback
//	    }
//	    return txDB.UpdateByID(user.ID, "status", "active") // Automatic commit
//	})
//
// Example - Complex transaction with query options:
//
//	err := database.Database[*model.Order](nil).Transaction(func(txDB types.Database[*model.Order]) error {
//	    // All query options work as expected
//	    if err := txDB.WithLock(consts.LockUpdate).Get(&order, orderID); err != nil {
//	        return err
//	    }
//	    order.Status = "processed"
//	    return txDB.Update(&order)
//	})
//
// Example - With custom rollback:
//
//	err := database.Database[*model.User](nil).WithRollback(func() {
//	    // Custom cleanup logic
//	}).Transaction(func(txDB types.Database[*model.User]) error {
//	    return txDB.Create(&user)
//	})
//
// For multi-model transactions, use TransactionFunc instead.
//
// Returns ErrNilTransactionFunc if fn is nil.
func (db *database[M]) Transaction(fn func(txDB types.Database[M]) error) error {
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

	begin := time.Now()

	return db.ins.Transaction(func(tx *gorm.DB) error {
		// Create a new database instance with transaction context
		txDB := Database[M](db.ctx).WithTx(tx)

		// Copy relevant options to the transaction database instance
		if db.rollbackFunc != nil {
			txDB = txDB.WithRollback(db.rollbackFunc)
		}

		// Execute the user function with the transaction database instance
		if err := fn(txDB); err != nil {
			// Execute custom rollback logic if provided
			if db.rollbackFunc != nil {
				db.rollbackFunc()
			}
			logger.Database.WithDatabaseContext(db.ctx, consts.Phase("Transaction")).Errorz(
				"transaction rolled back due to error",
				zap.Error(err),
				zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
			)
			return err
		}

		logger.Database.WithDatabaseContext(db.ctx, consts.Phase("Transaction")).Infoz(
			"transaction committed successfully",
			zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
		)
		return nil
	})
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
// - Operations that call WithTx(tx) are executed in the same transaction
//
// Relationship with other transaction methods:
// - Use Transaction: For single-model transactions (recommended, safer - auto WithTx)
// - Use TransactionFunc: For multi-model transactions (requires manual WithTx)
// - Use WithRollback: To add custom cleanup logic on transaction failure
//
// Example - Multi-model transaction:
//
//	err := database.Database[*model.User](nil).TransactionFunc(func(tx any) error {
//	    userDB := database.Database[*model.User](nil).WithTx(tx)    // Must use WithTx!
//	    orderDB := database.Database[*model.Order](nil).WithTx(tx)  // Must use WithTx!
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
//	err := database.Database[*model.Order](nil).TransactionFunc(func(tx any) error {
//	    orderDB := database.Database[*model.Order](nil).WithTx(tx)
//	    userDB := database.Database[*model.User](nil).WithTx(tx)
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
//	    userDB := database.Database[*model.User](nil)
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

	begin := time.Now()

	return db.ins.Transaction(func(tx *gorm.DB) error {
		// Execute the user function with the transaction gorm.DB instance
		if err := fn(tx); err != nil {
			// Execute custom rollback logic if provided
			if db.rollbackFunc != nil {
				db.rollbackFunc()
			}
			logger.Database.WithDatabaseContext(db.ctx, consts.Phase("TransactionFunc")).Errorz(
				"transaction rolled back due to error",
				zap.Error(err),
				zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
			)
			return err
		}

		logger.Database.WithDatabaseContext(db.ctx, consts.Phase("TransactionFunc")).Infoz(
			"transaction committed successfully",
			zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
		)
		return nil
	})
}
