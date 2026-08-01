package database

import (
	"context"
	"time"

	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Transaction executes fn within a database transaction and injects the
// transaction into the context passed to fn. Every database.Database[M](ctx)
// chain started from that context automatically joins the transaction; there
// is no manual binding step.
//
// If the provided ctx already carries a transaction, fn joins the outer
// transaction directly: no new transaction, span, or savepoint is created.
// This matches the boundary rule the write methods follow: the first explicit
// transaction owns the boundary, and everything inside shares it.
//
// Operations that must NOT join the transaction belong outside the closure:
// the closure body is the begin/commit block, so run them before calling
// Transaction or after it returns (for example, compensation writes on error).
// Work that must happen only once the transaction is durable belongs in
// AfterCommit instead, which runs it after the commit and skips it on rollback.
//
// Returns ErrNilTransaction if fn is nil, and an error marked with
// ErrAfterCommit when the transaction committed but a registered after-commit
// action failed. Panics if the database is not initialized, consistent with
// Database[M].
func Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return ErrNilTransaction
	}
	if DB() == nil || DB() == new(gorm.DB) {
		panic("database is not initialized")
	}
	return transactionOn(ctx, DB(), fn)
}

// TransactionOn is Transaction on an application-held database instance: fn
// runs inside a transaction opened on that instance, and only DatabaseOn
// chains for the same instance join it — a default-database chain inside fn
// keeps its own connection. Cross-instance atomicity is not provided. Panics
// on a nil instance, consistent with DatabaseOn.
func TransactionOn(ctx context.Context, instance *gorm.DB, fn func(ctx context.Context) error) error {
	if fn == nil {
		return ErrNilTransaction
	}
	if instance == nil {
		panic("database instance cannot be nil")
	}
	if isOpenTransaction(instance) {
		return ErrTransactionInstance
	}
	return transactionOn(ctx, instance, fn)
}

// transactionOn is the shared body of Transaction and TransactionOn. The
// connection handle keys the context transaction, so per-instance
// transactions coexist and joining is always same-instance only.
func transactionOn(ctx context.Context, base *gorm.DB, fn func(ctx context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := dbruntime.TxFromContext(ctx, base); ok {
		return fn(ctx)
	}

	spanCtx, span := gstotel.StartSpan(ctx, gstotel.OperationSpanName("database", "Transaction"))
	defer span.End()
	gstotel.AddSpanTags(span, map[string]any{
		"component":          "database",
		"database.operation": "Transaction",
	})

	begin := time.Now()
	// Deriving the closure context from spanCtx makes per-statement spans from
	// GormTracingPlugin nest under this transaction span, and the boundary makes
	// every chain opened on the same handle inside fn join gormTx while
	// collecting the actions to run once it commits.
	txErr := withTransactionBoundary(spanCtx, base, base.WithContext(spanCtx),
		func(txCtx context.Context, _ *gorm.DB) error {
			if err := fn(txCtx); err != nil {
				logger.Database.WithContext(ctx, consts.Phase("Transaction")).Errorz(
					"transaction rolled back due to error",
					zap.Error(err),
					util.LogDuration(time.Since(begin)),
				)
				return err
			}
			logger.Database.WithContext(ctx, consts.Phase("Transaction")).Infoz(
				"transaction committed successfully",
				util.LogDuration(time.Since(begin)),
			)
			return nil
		})

	// Recorded after the transaction returns so commit-phase failures are also
	// captured on the span.
	gstotel.RecordError(span, txErr)
	return txErr
}
