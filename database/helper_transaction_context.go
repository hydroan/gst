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

// withModelHookTransaction runs write hooks and the main write in one database
// transaction when this operation is responsible for creating the boundary.
//
// Create, Update, and Delete execute model hooks around the actual GORM write.
// Without this wrapper, a hook can update a second model and then fail after the
// primary model write has already committed. This helper makes the write phase
// atomic by creating one transaction for the whole hook/write/hook sequence and
// by storing that transaction in db.ctx before hooks are invoked.
//
// If db.ctx already carries a transaction, this method deliberately does not
// start a nested transaction. The caller is already inside an explicit
// Transaction/TransactionFunc or an outer model hook write, so all Database[T](ctx)
// calls should continue sharing the first transaction boundary.
//
// This helper binds the transaction with contextWithTx directly instead of going
// through WithTx, so the tracing span re-parenting done by WithTx never applies to
// this hook boundary: spans of hooks and nested writes keep the operation span
// created by db.trace as their parent instead of moving under a transaction span.
func (db *database[M]) withModelHookTransaction(fn func() error) error {
	if db.noHook || db.dryRun {
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
