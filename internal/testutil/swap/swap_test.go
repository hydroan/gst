package swap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueRestoresThePreviousValueOnCleanup(t *testing.T) {
	value := "before"

	t.Run("swapped for the subtest", func(t *testing.T) {
		Value(t, &value, "after")
		require.Equal(t, "after", value)
	})

	require.Equal(t, "before", value)
}

func TestValueKeepsTheTestOutOfParallelRuns(t *testing.T) {
	value := "before"
	Value(t, &value, "after")

	require.Panics(t, t.Parallel)
}

func TestValueRefusesATestAlreadyRunningInParallel(t *testing.T) {
	t.Parallel()

	value := "before"
	require.Panics(t, func() { Value(t, &value, "after") })
	require.Equal(t, "before", value)
}
