package database

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	gstotel "github.com/hydroan/gst/otel"
	"go.opentelemetry.io/otel/attribute"
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
// action failed. Returns ErrUnsupportedOnDialect on a ClickHouse instance,
// which has no transactions. Panics if the database is not initialized,
// consistent with Database[M].
func Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return ErrNilTransaction
	}
	if DB() == nil {
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
	// ClickHouse has no transactions, so a boundary opened on it could never
	// deliver the all-or-nothing promise this function makes; the entry fails
	// per the capability-miss rule instead of pretending.
	if dialectOf(base) == dialectClickHouse {
		return errors.Wrap(ErrUnsupportedOnDialect, "Transaction on clickhouse")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := dbruntime.TxFromContext(ctx, base); ok {
		return fn(ctx)
	}

	spanCtx, span := gstotel.StartSpan(ctx, gstotel.OperationSpanName("database", "Transaction"))
	defer span.End()
	// No recording gate here: both attributes are constants, so there is nothing
	// to skip building. Gate where assembling the attributes costs something —
	// see the per-operation batch in database[M].trace.
	span.SetAttributes(
		attribute.String("component", "database"),
		attribute.String("database.operation", "Transaction"),
	)

	// Deriving the closure context from spanCtx makes per-statement spans from
	// otelgorm nest under this transaction span, and the boundary makes
	// every chain opened on the same handle inside fn join the transaction
	// while collecting the actions to run once it commits.
	txErr := withTransactionBoundary(spanCtx, base, base.WithContext(spanCtx),
		func(txCtx context.Context, _ *gorm.DB) error {
			return fn(txCtx)
		})

	// Recorded after the transaction returns so commit-phase failures are also
	// captured on the span. The span is this function's only failure record:
	// logging the error belongs to the boundary that owns it (the controller
	// fallback for a request, the cronjob runner for a job), so no log line
	// is written here.
	gstotel.RecordError(span, txErr)
	return txErr
}
