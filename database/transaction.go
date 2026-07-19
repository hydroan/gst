package database

import (
	"context"
	"time"

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
// This matches withModelHookTransaction's boundary rule: the first explicit
// transaction owns the boundary, and everything inside shares it.
//
// Operations that must NOT join the transaction belong outside the closure:
// the closure body is the begin/commit block, so run them before calling
// Transaction or after it returns (for example, compensation writes on error).
//
// Returns ErrNilTransaction if fn is nil. Panics if the database is not
// initialized, consistent with Database[M].
func Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return ErrNilTransaction
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}
	if DB() == nil || DB() == new(gorm.DB) {
		panic("database is not initialized")
	}

	spanCtx, span := gstotel.StartSpan(ctx, gstotel.OperationSpanName("database", "Transaction"))
	defer span.End()
	gstotel.AddSpanTags(span, map[string]any{
		"component":          "database",
		"database.operation": "Transaction",
	})

	begin := time.Now()
	// Deriving the closure context from spanCtx makes per-statement spans from
	// GormTracingPlugin nest under this transaction span, and contextWithTx
	// makes every Database[M](ctx) chain inside fn join gormTx.
	txErr := DB().WithContext(spanCtx).Transaction(func(gormTx *gorm.DB) error {
		if err := fn(contextWithTx(spanCtx, gormTx)); err != nil {
			logger.Database.WithContext(ctx, consts.Phase("Transaction")).Errorz(
				"transaction rolled back due to error",
				zap.Error(err),
				zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
			)
			return err
		}
		logger.Database.WithContext(ctx, consts.Phase("Transaction")).Infoz(
			"transaction committed successfully",
			zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
		)
		return nil
	})

	// Recorded after the transaction returns so commit-phase failures are also
	// captured on the span.
	gstotel.RecordError(span, txErr)
	return txErr
}
