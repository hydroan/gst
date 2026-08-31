package database

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Cleanup permanently deletes all soft-deleted records of M from the default
// database. It removes every row whose deleted_at column is not null.
// WARNING: This is a destructive operation that cannot be undone.
//
// It is a package-level maintenance function rather than a chain method on
// purpose: it ignores query conditions, touches the whole table, and shares
// nothing with the CRUD contract. Panics if the database is not initialized,
// consistent with Database[M].
//
// On a ClickHouse instance it answers ErrUnsupportedOnDialect: the
// soft-delete regime does not exist there.
func Cleanup[M types.Model](ctx context.Context) error {
	if DB() == nil {
		panic("database is not initialized")
	}
	return cleanupOn[M](ctx, DB())
}

// CleanupOn is Cleanup on an application-held database instance. See
// DatabaseOn for the instance semantics, including the panic on nil.
func CleanupOn[M types.Model](ctx context.Context, instance *gorm.DB) error {
	if instance == nil {
		panic("database instance cannot be nil")
	}
	return cleanupOn[M](ctx, instance)
}

// cleanupOn is the shared body of Cleanup and CleanupOn.
func cleanupOn[M types.Model](ctx context.Context, base *gorm.DB) (err error) {
	db, ok := databaseFor[M](ctx, base).(*database[M])
	if !ok {
		// Unreachable while databaseFor returns the concrete chain, but
		// swallowing it would surface later as a nil dereference.
		return ErrInvalidDB
	}
	defer db.reset()

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

	tx := db.ins.Session(&gorm.Session{}).Limit(-1).Where("deleted_at IS NOT NULL").Model(*new(M)).Unscoped().Delete(make([]M, 0))
	// First-hand exit of a stack-less GORM/driver error; see the error-stack
	// contract in doc.go. WithStack passes nil through.
	return errors.WithStack(tx.Error)
}

// Health checks connectivity of the default database: a round-trip statement,
// a connection pool capacity warning, and a ping for response time.
//
// It is a package-level function because health is a property of the
// connection, not of any model: the former chain form borrowed a model type
// it never used. Today it checks the single default handle; once the database
// grows read replicas this is the entry that will cover every node.
//
// Returns nil if all checks pass. Panics if the database is not initialized,
// consistent with Database[M].
func Health(ctx context.Context) error {
	if DB() == nil {
		panic("database is not initialized")
	}
	return healthOn(ctx, DB())
}

// HealthOn is Health on an application-held database instance. See DatabaseOn
// for the instance semantics, including the panic on nil. Handing it an open
// transaction reports ErrTransactionInstance: health describes a connection
// pool, which a transaction is not.
func HealthOn(ctx context.Context, instance *gorm.DB) error {
	if instance == nil {
		panic("database instance cannot be nil")
	}
	if isOpenTransaction(instance) {
		return ErrTransactionInstance
	}
	return healthOn(ctx, instance)
}

// healthOn is the shared body of Health and HealthOn.
func healthOn(ctx context.Context, instance *gorm.DB) error {
	if ctx == nil {
		ctx = context.Background()
	}
	begin := time.Now()

	// 1.check database connection
	if err := instance.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
		logger.Database.WithContext(ctx, phaseHealth).Errorz(
			"database connection check failed",
			zap.Error(err),
			util.LogDuration(time.Since(begin)),
		)
		return errors.Wrap(err, "database connection check failed")
	}

	// 2.check database connection pool
	sqlDB, err := instance.DB()
	if err != nil {
		logger.Database.WithContext(ctx, phaseHealth).Errorz(
			"get sql.DB instance failed",
			zap.Error(err),
			util.LogDuration(time.Since(begin)),
		)
		return errors.Wrap(err, "get sql.DB instance failed")
	}

	// check database connection pool config
	stats := sqlDB.Stats()
	if stats.OpenConnections >= stats.MaxOpenConnections {
		logger.Database.WithContext(ctx, phaseHealth).Warnz(
			"database connection pool is full",
			zap.Int("open", stats.OpenConnections),
			zap.Int("max", stats.MaxOpenConnections),
			zap.Int("in_use", stats.InUse),
			zap.Int("idle", stats.Idle),
			util.LogDuration(time.Since(begin)),
		)
	}

	// 3.check database response time
	if err := sqlDB.PingContext(ctx); err != nil {
		logger.Database.WithContext(ctx, phaseHealth).Errorz(
			"database ping failed",
			zap.Error(err),
			util.LogDuration(time.Since(begin)),
		)
		return errors.Wrap(err, "database ping failed")
	}

	// A passing check is the expected outcome; probes may run it on a tight
	// interval, so success stays at debug level while failures log above.
	logger.Database.WithContext(ctx, phaseHealth).Debugz(
		"database health check passed",
		zap.Int("open", stats.OpenConnections),
		zap.Int("max", stats.MaxOpenConnections),
		zap.Int("in_use", stats.InUse),
		zap.Int("idle", stats.Idle),
		util.LogDuration(time.Since(begin)),
	)

	return nil
}
