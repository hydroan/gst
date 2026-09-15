package dbruntime

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestTransactionGuardIsOneSlot proves the guard slot holds one check: a
// second install panics instead of silently replacing the first, clearing
// the slot takes a nil, and with no guard installed a transaction passes.
func TestTransactionGuardIsOneSlot(t *testing.T) {
	t.Cleanup(func() { SetTransactionGuard(nil) })

	require.NoError(t, GuardTransaction(context.Background(), nil, nil), "no guard means no check")

	errGuard := errors.New("sample guard failure")
	SetTransactionGuard(func(context.Context, *gorm.DB, *gorm.DB) error { return errGuard })
	require.ErrorIs(t, GuardTransaction(context.Background(), nil, nil), errGuard)
	require.PanicsWithValue(t, "dbruntime: a transaction guard is already installed", func() {
		SetTransactionGuard(func(context.Context, *gorm.DB, *gorm.DB) error { return nil })
	})

	SetTransactionGuard(nil)
	require.NoError(t, GuardTransaction(context.Background(), nil, nil), "a cleared slot means no check")
}
