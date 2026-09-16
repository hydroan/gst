package dbruntime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestTxContextInstanceIsolation verifies that a transaction stored for one
// connection handle is invisible to lookups keyed by another, so a chain on
// the default database never joins another instance's transaction and vice
// versa.
func TestTxContextInstanceIsolation(t *testing.T) {
	analytics := &gorm.DB{}
	tx := &gorm.DB{}
	ctx := WithTx(context.Background(), tx, analytics)

	got, ok := TxFromContext(ctx, analytics)
	require.True(t, ok)
	require.Same(t, tx, got)

	primary := &gorm.DB{}
	_, ok = TxFromContext(ctx, primary)
	require.False(t, ok, "a lookup keyed by another handle must not see this transaction")

	primaryTx := &gorm.DB{}
	ctx = WithTx(ctx, primaryTx, primary)
	got, ok = TxFromContext(ctx, primary)
	require.True(t, ok)
	require.Same(t, primaryTx, got)
	got, ok = TxFromContext(ctx, analytics)
	require.True(t, ok, "per-instance transactions must coexist in one context tree")
	require.Same(t, tx, got)
}

// TestInTransactionSeesEveryInstance verifies the instance-agnostic view of a
// transaction context: a transaction open on any instance marks the context,
// for the callers that must not run inside a transaction whichever instance
// it is open on.
func TestInTransactionSeesEveryInstance(t *testing.T) {
	require.False(t, InTransaction(context.Background()))
	require.False(t, InTransaction(nil)) //nolint:staticcheck // nil is a supported input, mirroring TxFromContext.

	analytics := &gorm.DB{}
	require.True(t, InTransaction(WithTx(context.Background(), &gorm.DB{}, analytics)),
		"a transaction on any instance marks the context")
	require.False(t, InTransaction(WithTx(context.Background(), nil, analytics)),
		"no transaction, no mark")
}

// TestHandleResolvesTheContextTransaction covers the resolution the framework's
// database chain and its third-party adapters share: an operation runs on the
// context transaction of its own instance, and on the plain handle when there
// is none.
func TestHandleResolvesTheContextTransaction(t *testing.T) {
	primary := &gorm.DB{}
	analytics := &gorm.DB{}
	tx := &gorm.DB{}

	require.Same(t, primary, Handle(context.Background(), primary),
		"outside a transaction the instance itself is the connection")

	ctx := WithTx(context.Background(), tx, primary)
	require.Same(t, tx, Handle(ctx, primary), "an operation must join its instance's transaction")
	require.Same(t, analytics, Handle(ctx, analytics),
		"another instance must not be pulled into this transaction")

	require.Panics(t, func() { Handle(context.Background(), nil) })
}
