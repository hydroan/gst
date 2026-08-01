package database

import (
	"context"
	"sync"

	"github.com/cockroachdb/errors"
)

// transactionBoundary collects the actions registered to run once the
// transaction that owns it has committed.
type transactionBoundary struct {
	mu      sync.Mutex
	actions []func(context.Context) error
}

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
// failed revoke would apply the grant on top of state the revoke was supposed to
// have cleared, which is the wrong side to fail on; stopping leaves less access
// in place, not more.
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
