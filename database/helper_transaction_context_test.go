package database

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
	ctx := contextWithTx(context.Background(), tx, analytics)

	got, ok := txFromContext(ctx, analytics)
	require.True(t, ok)
	require.Same(t, tx, got)

	primary := &gorm.DB{}
	_, ok = txFromContext(ctx, primary)
	require.False(t, ok, "a lookup keyed by another handle must not see this transaction")

	primaryTx := &gorm.DB{}
	ctx = contextWithTx(ctx, primaryTx, primary)
	got, ok = txFromContext(ctx, primary)
	require.True(t, ok)
	require.Same(t, primaryTx, got)
	got, ok = txFromContext(ctx, analytics)
	require.True(t, ok, "per-instance transactions must coexist in one context tree")
	require.Same(t, tx, got)
}
