package database

import (
	"context"

	"gorm.io/gorm"
)

type transactionContextKey struct{}

// contextWithTx returns a child context carrying the current GORM transaction.
//
// Model hooks only receive context.Context. They do not receive the database
// wrapper or the raw *gorm.DB transaction, and that is intentional: model code
// should keep using the framework entry point, for example
// database.Database[*Config](ctx).Update(config). The transaction therefore has
// to travel through the hook context. Database[M](ctx) reads this value back and
// binds the returned operation chain to the same transaction.
//
// The transaction value is scoped to this context tree only. It is not global,
// does not cross requests, and is lost if hook code replaces the context with
// context.Background().
func contextWithTx(ctx context.Context, tx *gorm.DB) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tx == nil {
		return ctx
	}
	return context.WithValue(ctx, transactionContextKey{}, tx)
}

func txFromContext(ctx context.Context) (*gorm.DB, bool) {
	if ctx == nil {
		return nil, false
	}
	tx, ok := ctx.Value(transactionContextKey{}).(*gorm.DB)
	if !ok || tx == nil {
		return nil, false
	}
	return tx, true
}

// withWriteTransaction runs a write operation (hooks plus the main write) in
// one database transaction when this operation is responsible for creating the
// boundary.
//
// Every write path (Create, Update, Upsert, Delete) needs this boundary, with
// or without model hooks:
//   - Model hooks can update a second model; without the boundary a hook could
//     fail after the primary write already committed.
//   - Multi-row and multi-batch writes must be all-or-nothing; without the
//     boundary a mid-loop failure would leave earlier rows committed. This
//     also holds for WithoutHook chains, which is why noHook does not skip
//     the transaction: it only skips hook invocation inside fn.
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
	if _, ok := txFromContext(db.ctx); ok {
		return fn()
	}

	parentCtx := db.ctx
	parentIns := db.ins
	return db.ins.Transaction(func(tx *gorm.DB) error {
		txCtx := contextWithTx(parentCtx, tx)
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
