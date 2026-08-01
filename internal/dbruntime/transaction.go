package dbruntime

import (
	"context"

	"gorm.io/gorm"
)

// transactionContextKey carries the transaction opened on one connection
// handle.
//
// The key holds the handle the transaction was opened on, so transactions on
// different database instances coexist in one context tree and a lookup only
// ever finds the transaction of its own instance: same handle, same transaction
// view.
type transactionContextKey struct{ base *gorm.DB }

// WithTx returns a child context carrying tx as the transaction open on base.
//
// Model hooks only receive a context.Context. They do not receive the database
// wrapper or the raw *gorm.DB transaction, and that is intentional: model code
// should keep using the framework entry point, for example
// database.Database[*Config](ctx).Update(config). The transaction therefore has
// to travel through the hook context, and the database chain reads it back to
// bind itself to the same transaction.
//
// The value is scoped to this context tree only. It is not global, does not
// cross requests, and is lost if code replaces the context with
// context.Background().
func WithTx(ctx context.Context, tx *gorm.DB, base *gorm.DB) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tx == nil {
		return ctx
	}
	return context.WithValue(ctx, transactionContextKey{base: base}, tx)
}

// TxFromContext returns the transaction ctx carries for base, if any.
func TxFromContext(ctx context.Context, base *gorm.DB) (*gorm.DB, bool) {
	if ctx == nil {
		return nil, false
	}
	tx, ok := ctx.Value(transactionContextKey{base: base}).(*gorm.DB)
	if !ok || tx == nil {
		return nil, false
	}
	return tx, true
}

// Handle returns the connection an operation on instance must run through for
// ctx: the transaction ctx carries for that instance when there is one, and
// instance itself otherwise.
//
// It is internal on purpose. Application code reaches the database through
// database.Database[M](ctx), which joins the context transaction already; the
// only callers that need a raw *gorm.DB are framework integrations wrapping a
// third-party library whose storage layer can be handed nothing else. Exporting
// it would offer a second way to reach the database that looks equivalent to
// the first and silently is not, because a handle kept past the call resumes
// writing outside the caller's transaction.
func Handle(ctx context.Context, instance *gorm.DB) *gorm.DB {
	if instance == nil {
		panic("database instance cannot be nil")
	}
	if tx, ok := TxFromContext(ctx, instance); ok {
		return tx
	}
	return instance
}
