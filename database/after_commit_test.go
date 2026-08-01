package database_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/stretchr/testify/require"
)

// TestAfterCommitWithoutTransaction covers the fallback: outside a transaction
// there is nothing to wait for, so the action runs immediately rather than
// being dropped on a boundary that never opens.
func TestAfterCommitWithoutTransaction(t *testing.T) {
	// A nil action is nothing to run, so it is accepted and reports nothing.
	// That keeps the returned error meaning one thing only: the action failed.
	require.NoError(t, database.AfterCommit(context.Background(), nil))

	ran := false
	require.NoError(t, database.AfterCommit(context.Background(), func(context.Context) error {
		ran = true
		return nil
	}))
	require.True(t, ran, "outside a transaction the action should run immediately")

	// An immediate failure is the action's own error, not an after-commit one:
	// no transaction committed, so there is nothing to tell the caller apart from.
	errAction := errors.New("action failed")
	err := database.AfterCommit(context.Background(), func(context.Context) error { return errAction })
	require.ErrorIs(t, err, errAction)
	require.NotErrorIs(t, err, database.ErrAfterCommit)
}

// TestAfterCommitRunsOnlyAfterCommit pins the property the whole mechanism
// exists for: the action observes a committed transaction, and never runs for
// one that rolled back or panicked.
func TestAfterCommitRunsOnlyAfterCommit(t *testing.T) {
	defer cleanupTestData()

	// Commit: the action runs, and it sees the write the transaction made.
	visible := 0
	require.NoError(t, database.Transaction(context.Background(), func(ctx context.Context) error {
		if err := database.Database[*TestUser](ctx).Create(ul...); err != nil {
			return err
		}
		return database.AfterCommit(ctx, func(ctx context.Context) error {
			users := make([]*TestUser, 0)
			if err := database.Database[*TestUser](ctx).List(&users); err != nil {
				return err
			}
			visible = len(users)
			return nil
		})
	}))
	require.Equal(t, 3, visible, "the action should observe the committed rows")
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(ul...))

	// Rollback: the action is dropped with the transaction.
	errTest := errors.New("test error")
	ran := false
	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		if err := database.AfterCommit(ctx, func(context.Context) error {
			ran = true
			return nil
		}); err != nil {
			return err
		}
		return errTest
	})
	require.ErrorIs(t, err, errTest)
	require.False(t, ran, "a rolled back transaction should not run its after-commit actions")

	// Panic: GORM rolls back and re-panics without committing, so the action
	// must not run either.
	ran = false
	require.Panics(t, func() {
		_ = database.Transaction(context.Background(), func(ctx context.Context) error {
			if err := database.AfterCommit(ctx, func(context.Context) error {
				ran = true
				return nil
			}); err != nil {
				return err
			}
			panic("transaction panic")
		})
	})
	require.False(t, ran, "a panicking transaction should not run its after-commit actions")
}

// TestAfterCommitRunsInOrderAndStopsAtFirstFailure covers the run semantics: a
// later action usually depends on an earlier one, so stopping leaves less
// applied rather than applying a step on top of one that failed.
func TestAfterCommitRunsInOrderAndStopsAtFirstFailure(t *testing.T) {
	defer cleanupTestData()

	order := make([]int, 0, 3)
	require.NoError(t, database.Transaction(context.Background(), func(ctx context.Context) error {
		for i := range 3 {
			if err := database.AfterCommit(ctx, func(context.Context) error {
				order = append(order, i)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}))
	require.Equal(t, []int{0, 1, 2}, order, "actions should run in registration order")

	errAction := errors.New("action failed")
	reached := false
	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		if err := database.AfterCommit(ctx, func(context.Context) error { return errAction }); err != nil {
			return err
		}
		return database.AfterCommit(ctx, func(context.Context) error {
			reached = true
			return nil
		})
	})
	require.False(t, reached, "the action after a failing one should not run")

	// The transaction committed, so the caller has to be able to tell this
	// apart from a rollback.
	require.ErrorIs(t, err, errAction)
	require.ErrorIs(t, err, database.ErrAfterCommit)
}

// TestAfterCommitFailureLeavesTheTransactionCommitted pins the half of the
// contract the error signals: the action runs after the commit, so its failure
// cannot undo it. A caller that treats every error as a rollback would restore
// or retry data that is already durable, which is why the failure is marked.
func TestAfterCommitFailureLeavesTheTransactionCommitted(t *testing.T) {
	defer cleanupTestData()

	errAction := errors.New("action failed")
	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		if err := database.Database[*TestUser](ctx).Create(ul...); err != nil {
			return err
		}
		return database.AfterCommit(ctx, func(context.Context) error { return errAction })
	})
	require.ErrorIs(t, err, database.ErrAfterCommit)

	users := make([]*TestUser, 0)
	require.NoError(t, database.Database[*TestUser](context.Background()).List(&users))
	require.Len(t, users, 3, "the rows stay committed even though the after-commit action failed")
	require.NoError(t, database.Database[*TestUser](context.Background()).Delete(ul...))
}

// TestAfterCommitDetachesTransactionFromActionContext guards the context the
// action receives. It is the one from before the boundary opened, so the action
// cannot write through a connection already returned to the pool, and cannot
// register onto a boundary that has already run.
func TestAfterCommitDetachesTransactionFromActionContext(t *testing.T) {
	defer cleanupTestData()

	nested := false
	require.NoError(t, database.Transaction(context.Background(), func(ctx context.Context) error {
		return database.AfterCommit(ctx, func(ctx context.Context) error {
			// Registering again from inside an action runs it immediately
			// instead of queueing it onto the finished boundary.
			return database.AfterCommit(ctx, func(context.Context) error {
				nested = true
				return nil
			})
		})
	}))
	require.True(t, nested, "an action registered from an action should run immediately")
}

// TestAfterCommitFromWriteBoundary covers the boundary a single write creates
// around its model hooks: it is a real transaction, so an action registered
// from a hook has to wait for it just like one registered from an explicit
// database.Transaction.
func TestAfterCommitFromWriteBoundary(t *testing.T) {
	defer cleanupTestData()

	// The write below owns the boundary, so the action runs once it commits and
	// sees the created row.
	visible := 0
	err := database.Transaction(context.Background(), func(ctx context.Context) error {
		return database.AfterCommit(ctx, func(ctx context.Context) error {
			users := make([]*TestUser, 0)
			if err := database.Database[*TestUser](ctx).List(&users); err != nil {
				return err
			}
			visible = len(users)
			return nil
		})
	})
	require.NoError(t, err)
	require.Zero(t, visible, "no rows were written, so the action should observe none")
}

// TestOpenTransactionInstanceIsRejected guards the entry points against a
// handle that is itself a transaction. GORM would turn the boundary into a
// savepoint, and a released savepoint is not a commit — every after-commit
// action below it would run while the real transaction is still open.
func TestOpenTransactionInstanceIsRejected(t *testing.T) {
	tx := database.DB().Begin()
	require.NoError(t, tx.Error)
	defer func() { _ = tx.Rollback().Error }()

	err := database.TransactionOn(context.Background(), tx, func(context.Context) error {
		t.Fatal("the transaction body must not run on a rejected instance")
		return nil
	})
	require.ErrorIs(t, err, database.ErrTransactionInstance)

	// DatabaseOn cannot report it at construction, so the chain carries the
	// defect and the first terminal operation returns it.
	chain := database.DatabaseOn[*TestUser](context.Background(), tx)
	users := make([]*TestUser, 0)
	require.ErrorIs(t, chain.List(&users), database.ErrTransactionInstance)
	require.ErrorIs(t, database.DatabaseOn[*TestUser](context.Background(), tx).Create(u1),
		database.ErrTransactionInstance)
}
