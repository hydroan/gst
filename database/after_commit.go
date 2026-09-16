package database

import (
	"context"
)

// AfterCommit registers fn to run after the transaction ctx is inside commits,
// and runs it immediately when ctx is inside no transaction.
//
// It exists for effects that must not become visible before the data they
// describe is durable, and that a rollback cannot take back: process-local
// state, cache invalidation, an outbound notification. Doing that work inside
// the transaction leaves it applied after a rollback; doing it after the
// transaction returns, without this, means doing it after a rollback too.
//
// fn receives the context from before the transaction opened. That context
// carries neither the transaction nor this boundary, so fn cannot write through
// a connection already returned to the pool, and cannot register a further
// action on a boundary that has already run.
//
// Registration order is run order, and the first failure stops the rest. A
// failure is returned to whoever called Transaction or the write method that
// owns the boundary, marked with ErrAfterCommit: the transaction itself has
// committed, so the caller must not treat that error as a rollback.
//
// A nil action registers nothing and reports no error. The returned error
// therefore always comes from running an action, never from the arguments,
// which is what lets a caller read a non-nil result as "the effect failed"
// without first ruling out its own call.
func AfterCommit(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return nil
	}

	boundary, ok := boundaryFromContext(ctx)
	if !ok {
		// Outside a transaction there is nothing to wait for, and deferring the
		// action to a boundary that will never open would drop it.
		return fn(ctx)
	}
	boundary.add(fn)
	return nil
}
