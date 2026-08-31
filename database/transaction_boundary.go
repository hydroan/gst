package database

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"gorm.io/gorm"
)

// boundaryContextKey carries the after-commit boundary of the transaction the
// context is inside.
//
// Unlike transactionContextKey it is not keyed by connection handle: a caller
// asking to run something after "this" transaction commits is not choosing a
// database, and requiring it to name one would make the question harder than it
// is. A context nested inside two boundaries on different instances resolves to
// the innermost, which is the transaction the caller is actually writing in.
type boundaryContextKey struct{}

func contextWithBoundary(ctx context.Context, boundary *transactionBoundary) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, boundaryContextKey{}, boundary)
}

func boundaryFromContext(ctx context.Context) (*transactionBoundary, bool) {
	if ctx == nil {
		return nil, false
	}
	boundary, ok := ctx.Value(boundaryContextKey{}).(*transactionBoundary)
	if !ok || boundary == nil {
		return nil, false
	}
	return boundary, true
}

// withTransactionBoundary opens a transaction on ins, runs fn inside it, and
// then — only once that transaction has committed — runs the after-commit
// actions fn registered.
//
// Every transaction gst opens goes through here, so that "after commit" means
// the same thing for an explicit database.Transaction and for the boundary a
// single write creates around its hooks.
//
// The actions run after ins.Transaction returns rather than in a defer. GORM
// rolls back and re-panics without committing, so a deferred call would run
// them for a transaction that never happened.
//
// They receive ctx, the context from before the boundary opened, which carries
// neither the transaction nor the boundary.
func withTransactionBoundary(
	ctx context.Context,
	base *gorm.DB,
	ins *gorm.DB,
	fn func(txCtx context.Context, tx *gorm.DB) error,
) error {
	boundary := new(transactionBoundary)
	if err := ins.Transaction(func(tx *gorm.DB) error {
		return fn(contextWithBoundary(dbruntime.WithTx(ctx, tx, base), boundary), tx)
	}); err != nil {
		// The stack embedded here covers BEGIN/COMMIT failures, which GORM
		// hands back without one. An error from fn usually already carries a
		// deeper stack from its own first-hand exit; the shallower one added
		// here is ignored by the deepest-stack rule (see the error-stack
		// contract in doc.go).
		return errors.WithStack(err)
	}
	return boundary.run(ctx)
}

// isOpenTransaction reports whether instance is itself an in-flight transaction
// rather than a connection to open one on.
//
// GORM turns a transaction opened on a transaction into a savepoint, and a
// savepoint being released is not a commit. Every boundary below would then run
// its after-commit actions while the real transaction is still open, and an
// outer rollback could not take them back. Rejecting such a handle at the entry
// point keeps "this call owns a real boundary" true everywhere inside, instead
// of leaving each layer to work out whether it does.
func isOpenTransaction(instance *gorm.DB) bool {
	if instance == nil || instance.Statement == nil {
		return false
	}
	committer, ok := instance.Statement.ConnPool.(gorm.TxCommitter)
	return ok && committer != nil
}

// withWriteTransaction runs a write operation (hooks plus the main write) in
// one database transaction when this operation is responsible for creating the
// boundary.
//
// A write path (Create, Update, Upsert, Delete) needs this boundary whenever
// it has more than the bare statement to keep atomic:
//   - Model hooks can update a second model; without the boundary a hook could
//     fail after the primary write already committed.
//   - Multi-row and multi-batch writes must be all-or-nothing; without the
//     boundary a mid-loop failure would leave earlier rows committed. This
//     also holds for WithoutHook chains: noHook only skips hook invocation
//     inside fn, never the boundary of a multi-statement write.
//
// A write with neither concern — no overridden hooks (or WithoutHook) and
// exactly one SQL statement — takes withSingleStatementWrite instead.
//
// WithDryRun skips the boundary because it performs no database I/O.
//
// If db.ctx already carries a transaction, this method deliberately does not
// start a nested transaction. The caller is already inside an explicit
// database.Transaction or an outer model hook write, so all Database[T](ctx)
// calls should continue sharing the first transaction boundary.
//
// Unlike database.Transaction, this write boundary does not create its own
// span: spans of hooks and nested writes keep the operation span created by
// db.trace as their parent instead of moving under a transaction span.
func (db *database[M]) withWriteTransaction(fn func() error) error {
	if db.dryRun {
		return fn()
	}
	if _, ok := dbruntime.TxFromContext(db.ctx, db.base); ok {
		return fn()
	}

	parentCtx := db.ctx
	parentIns := db.ins
	return withTransactionBoundary(parentCtx, db.base, db.ins, func(txCtx context.Context, tx *gorm.DB) error {
		db.ctx = txCtx
		db.ins = tx.Session(&gorm.Session{
			SkipDefaultTransaction: false,
			NewDB:                  false,
		}).WithContext(txCtx)
		defer func() {
			db.ctx = parentCtx
			db.ins = parentIns
		}()

		return fn()
	})
}

// withSingleStatementWrite runs a write that needs no transaction of its own:
// none of the operation's model hooks is overridden (or the caller chose
// WithoutHook) and the operation compiles to exactly one SQL statement, whose
// own atomicity already makes it all-or-nothing. Paying a BEGIN/COMMIT pair
// around it would buy nothing, so the write runs with GORM's default
// per-statement transaction switched off.
//
// An ambient transaction still wins: when db.ctx carries one, db.ins is
// already that transaction's handle and the write joins it unchanged, exactly
// as in withWriteTransaction. Without a boundary, AfterCommit falls back to
// its documented immediate-execution semantics — and no-op hooks register
// nothing, so nothing is lost.
func (db *database[M]) withSingleStatementWrite(fn func() error) error {
	if _, ok := dbruntime.TxFromContext(db.ctx, db.base); ok {
		return fn()
	}
	parentIns := db.ins
	db.ins = db.ins.Session(&gorm.Session{SkipDefaultTransaction: true}).WithContext(db.ctx)
	defer func() { db.ins = parentIns }()
	return fn()
}
