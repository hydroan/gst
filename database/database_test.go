package database_test

import (
	"context"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/stretchr/testify/require"
)

// TestNilContextIsRejected guards every entry point against a nil context:
// the context is what carries the transaction, the lease and the identity of
// the work down to the statements, so nothing may quietly run on a context
// of the framework's own. A chain or a union built on nil reports it from
// its first terminal operation, the way it reports its other defects; the
// entry points that run at once report it at once.
func TestNilContextIsRejected(t *testing.T) {
	// A variable, not a literal: the vet check on a literal nil context is
	// exactly the mistake this test makes on purpose.
	var nilCtx context.Context

	users := make([]*TestUser, 0)
	require.ErrorIs(t, database.Database[*TestUser](nilCtx).List(&users), database.ErrNilContext)
	require.ErrorIs(t, database.Database[*TestUser](nilCtx).Create(u1), database.ErrNilContext)
	require.ErrorIs(t, database.DatabaseOn[*TestUser](nilCtx, database.DB()).List(&users), database.ErrNilContext)

	require.ErrorIs(t, database.Transaction(nilCtx, func(context.Context) error {
		t.Fatal("the transaction body must not run on a nil context")
		return nil
	}), database.ErrNilContext)
	require.ErrorIs(t, database.TransactionOn(nilCtx, database.DB(), func(context.Context) error {
		t.Fatal("the transaction body must not run on a nil context")
		return nil
	}), database.ErrNilContext)

	rows := make([]TestUser, 0)
	require.ErrorIs(t, database.UnionAll[TestUser](nilCtx).Scan(&rows), database.ErrNilContext)

	require.ErrorIs(t, database.Health(nilCtx), database.ErrNilContext)
	require.ErrorIs(t, database.HealthOn(nilCtx, database.DB()), database.ErrNilContext)
}
