package database

import (
	"context"
	"database/sql"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// transactionBoundary collects the actions registered to run once the
// transaction that owns it has committed.
type transactionBoundary struct {
	mu      sync.Mutex
	actions []func(context.Context) error
}

// add registers fn to run once the transaction commits.
func (b *transactionBoundary) add(fn func(context.Context) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.actions = append(b.actions, fn)
}

// run executes the registered actions in registration order and stops at the
// first failure.
//
// Stopping is deliberate. Actions are registered by the same write that
// produced them, so a later one usually depends on an earlier one having
// happened — a revoke followed by a re-grant, for example. Continuing past a
// failed revoke would apply the grant on top of state the revoke was supposed
// to have cleared, which is the wrong side to fail on; stopping leaves less
// access in place, not more.
//
// The list is taken under the lock and cleared, so an action that registers
// another one cannot extend the run it is already part of.
func (b *transactionBoundary) run(ctx context.Context) error {
	b.mu.Lock()
	actions := b.actions
	b.actions = nil
	b.mu.Unlock()

	for _, action := range actions {
		if err := action(ctx); err != nil {
			// Joined rather than marked so that errors.Is finds both the
			// sentinel and the action's own error, whichever the caller
			// matches on.
			return errors.Join(ErrAfterCommit, err)
		}
	}
	return nil
}

// boundaryContextKey carries the after-commit boundary of the transaction the
// context is inside.
//
// Unlike transactionContextKey it is not keyed by connection handle: a caller
// asking to run something after "this" transaction commits is not choosing a
// database, and requiring it to name one would make the question harder than it
// is. A context nested inside two boundaries on different instances resolves to
// the innermost, which is the transaction the caller is actually writing in.
type boundaryContextKey struct{}

// boundaryFromContext returns the after-commit boundary ctx carries, and false
// when it carries none.
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
	// The transaction opens on the primary whatever the chain reads from: a
	// chain of a PreferReplica model carries the replica as its read side,
	// and dbresolver routes a statement back to the primary only outside a
	// transaction, so a transaction opened on that chain would run its writes
	// on the replica. Without replicas the clause names the one pool there is.
	if err := ins.Clauses(dbresolver.Write).Transaction(func(tx *gorm.DB) error {
		// gorm opens the transaction on the chain's own handle and copies the
		// chain's statement into it: Begin starts a fresh statement only on a
		// handle no chain has touched. The operation itself runs on tx and
		// keeps its own conditions. The context instead hands hooks and nested
		// chains what Begin gives a handle no chain has touched: a new
		// statement on the same transaction, so a chain they build starts from
		// none of the operation's conditions, selections or preloads.
		// Initialized starts that statement now: a NewDB session alone starts
		// it at the next chain call, and WithContext, the first call a chain
		// makes, copies the old one. The outer NewDB session keeps the handle
		// reusable.
		fresh := tx.Session(&gorm.Session{NewDB: true, Initialized: true}).Session(&gorm.Session{NewDB: true})
		txCtx := dbruntime.WithTx(ctx, fresh, base)
		txCtx = context.WithValue(txCtx, boundaryContextKey{}, boundary)
		return fn(txCtx, tx)
	}); err != nil {
		// A context that ends while its transaction is open has the standard
		// library roll the transaction back, and the commit after that
		// reports the transaction done rather than the ending that undid it —
		// or the ending itself, when the commit comes first. The ending is
		// the reason either way, and work told to stop must be able to tell
		// its context's ending from a failure of its own.
		if ctx.Err() != nil && errors.Is(err, sql.ErrTxDone) {
			err = ctx.Err()
		}
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
