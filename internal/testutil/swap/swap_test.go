package swap_test

import (
	"testing"

	"github.com/hydroan/gst/internal/testutil/swap"
	"github.com/stretchr/testify/require"
)

func TestValueRestoresThePreviousValueOnCleanup(t *testing.T) {
	value := "before"

	t.Run("swapped for the subtest", func(t *testing.T) {
		swap.Value(t, &value, "after")
		require.Equal(t, "after", value)
	})

	require.Equal(t, "before", value)
}

func TestValueKeepsTheTestOutOfParallelRuns(t *testing.T) {
	value := "before"
	swap.Value(t, &value, "after")

	require.Panics(t, t.Parallel)
}

func TestValueRefusesATestAlreadyRunningInParallel(t *testing.T) {
	t.Parallel()

	value := "before"
	require.Panics(t, func() { swap.Value(t, &value, "after") })
	require.Equal(t, "before", value)
}
