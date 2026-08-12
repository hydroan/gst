package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSwapValueRestoresThePreviousValueOnCleanup(t *testing.T) {
	value := "before"

	t.Run("swapped for the subtest", func(t *testing.T) {
		SwapValue(t, &value, "after")
		require.Equal(t, "after", value)
	})

	require.Equal(t, "before", value)
}
