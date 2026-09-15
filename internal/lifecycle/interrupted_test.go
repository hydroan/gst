package lifecycle

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// TestInterruptedCountsOnlyTheContextsOwnEnding pins what work returning an
// error counts as interrupted: the context's own ending, wrapped or joined
// with itself, and nothing that carries a failure of the work's own.
func TestInterruptedCountsOnlyTheContextsOwnEnding(t *testing.T) {
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	failure := errors.New("sample failure")

	require.False(t, Interrupted(context.Background(), context.Canceled), "a context still running ended nothing")
	require.False(t, Interrupted(ended, nil), "work that returned nothing was not interrupted")
	require.True(t, Interrupted(ended, ended.Err()))
	require.True(t, Interrupted(ended, errors.Wrap(ended.Err(), "query")), "the ending wrapped is still the ending")
	require.True(t, Interrupted(ended, errors.Join(ended.Err(), errors.Wrap(ended.Err(), "twice"))), "the ending joined with itself is still the ending")
	require.False(t, Interrupted(ended, failure), "a failure of the work's own is not an interruption")
	require.False(t, Interrupted(ended, errors.Join(ended.Err(), failure)), "a failure beside the ending must not hide behind it")
	require.False(t, Interrupted(ended, errors.Wrap(errors.Join(ended.Err(), failure), "round")), "nor when the join is wrapped")
}
