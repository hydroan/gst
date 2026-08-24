package database

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Cleanup permanently deletes all soft-deleted records from the database.
// This operation removes records where 'deleted_at' column is not null.
// WARNING: This is a destructive operation that cannot be undone.
//
// Returns database errors if the cleanup operation fails.
//
// Features:
//   - Permanently removes soft-deleted records
//   - Uses unscoped delete to bypass soft delete protection
//   - Applies to all records in the table (ignores query conditions)
//   - Helps maintain database performance by removing obsolete data
//   - WithDryRun builds cleanup SQL without permanently removing rows
//
// Example:
//
//	Cleanup()  // Remove all soft-deleted records
//	WithDryRun().Cleanup()  // Build cleanup SQL without removing records
//
// Note: This operation affects the entire table and ignores any previously
// set query conditions. Use with caution in production environments.
// On a ClickHouse instance it answers ErrUnsupportedOnDialect: the soft-delete
// regime does not exist there.
func (db *database[M]) Cleanup() (err error) {
	defer db.reset()

	if db.includeDeleted {
		return errors.Wrap(ErrWithDeletedOnWrite, "Cleanup")
	}
	if err = db.prepare(); err != nil {
		return err
	}
	// The deleted_at soft-delete regime does not exist on ClickHouse — its own
	// lightweight delete already is a mark-then-merge removal — so there are no
	// framework-made soft-deleted rows to purge there; rows mirrored from an
	// OLTP source are operations territory. Fails per the capability-miss rule.
	if db.dialect() == dialectClickHouse {
		return errors.Wrap(ErrUnsupportedOnDialect, "Cleanup on clickhouse")
	}
	done, _ := db.trace(phaseCleanup)
	defer func() { done(err) }()

	tx := db.ins.Session(&gorm.Session{DryRun: db.dryRun}).Limit(-1).Where("deleted_at IS NOT NULL").Model(*new(M)).Unscoped().Delete(make([]M, 0))
	if db.dryRun {
		return db.collectSQL(tx)
	}
	return tx.Error
}

// Health performs comprehensive database health checks including connectivity,
// connection pool status, and response time validation.
// Returns nil if all checks pass, otherwise returns detailed error information.
//
// Health checks performed:
//  1. Database connection test with SELECT 1 query
//  2. Connection pool status and capacity validation
//  3. Database ping test for response time measurement
//
// Returns database errors if any health check fails.
//
// Features:
//   - Comprehensive connectivity validation
//   - Connection pool monitoring and warnings
//   - Response time measurement
//   - Detailed logging of health status and metrics
//
// Example:
//
//	if err := Database[User]().Health(); err != nil {
//	  log.Fatal("Database unhealthy:", err)
//	}
func (db *database[M]) Health() error {
	defer db.reset()

	if err := db.prepare(); err != nil {
		return err
	}
	if db.buildingSQL {
		tx := dryRunSession(db.ins).Exec("SELECT 1")
		return db.collectSQL(tx)
	}

	begin := time.Now()

	// 1.check database connection
	if err := db.ins.Exec("SELECT 1").Error; err != nil {
		logger.Database.WithContext(db.ctx, phaseHealth).Errorz(
			"database connection check failed",
			zap.Error(err),
			util.LogDuration(time.Since(begin)),
		)
		return fmt.Errorf("database connection check failed: %w", err)
	}

	// 2.check database connection pool
	sqlDB, err := db.ins.DB()
	if err != nil {
		logger.Database.WithContext(db.ctx, phaseHealth).Errorz(
			"get sql.DB instance failed",
			zap.Error(err),
			util.LogDuration(time.Since(begin)),
		)
		return fmt.Errorf("get sql.DB instance failed: %w", err)
	}

	// check database connection pool config
	stats := sqlDB.Stats()
	if stats.OpenConnections >= stats.MaxOpenConnections {
		logger.Database.WithContext(db.ctx, phaseHealth).Warnz(
			"database connection pool is full",
			zap.Int("open", stats.OpenConnections),
			zap.Int("max", stats.MaxOpenConnections),
			zap.Int("in_use", stats.InUse),
			zap.Int("idle", stats.Idle),
			util.LogDuration(time.Since(begin)),
		)
	}

	// 3.check database response time
	ctx := context.Background()
	if db.ctx != nil {
		ctx = db.ctx
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		logger.Database.WithContext(db.ctx, phaseHealth).Errorz(
			"database ping failed",
			zap.Error(err),
			util.LogDuration(time.Since(begin)),
		)
		return fmt.Errorf("database ping failed: %w", err)
	}

	// A passing check is the expected outcome; probes may run it on a tight
	// interval, so success stays at debug level while failures log above.
	logger.Database.WithContext(db.ctx, phaseHealth).Debugz(
		"database health check passed",
		zap.Int("open", stats.OpenConnections),
		zap.Int("max", stats.MaxOpenConnections),
		zap.Int("in_use", stats.InUse),
		zap.Int("idle", stats.Idle),
		util.LogDuration(time.Since(begin)),
	)

	return nil
}
