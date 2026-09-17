package lifecycle

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// TestInterruptedCountsOnlyTheContextsOwnEnding pins what work returning an
// error counts as interrupted: the context's own ending — its error, or the
// cause it was ended with — wrapped or joined with itself, and nothing that
// carries a failure of the work's own.
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

	// A sentinel built with a stack, the way the framework's own are.
	cause := errors.New("sample cause")
	endedWithCause, cancelWithCause := context.WithCancelCause(context.Background())
	cancelWithCause(cause)

	require.True(t, Interrupted(endedWithCause, endedWithCause.Err()), "the error of a context ended with a cause is its ending")
	require.True(t, Interrupted(endedWithCause, context.Cause(endedWithCause)), "so is the cause")
	require.True(t, Interrupted(endedWithCause, errors.Wrap(context.Cause(endedWithCause), "query")), "the cause wrapped is still the ending")
	require.True(t, Interrupted(endedWithCause, errors.Join(endedWithCause.Err(), context.Cause(endedWithCause))), "the error joined with the cause is still the ending")
	require.False(t, Interrupted(endedWithCause, errors.Join(context.Cause(endedWithCause), failure)), "a failure beside the cause must not hide behind it")
	require.False(t, Interrupted(ended, cause), "the cause of another context is not this one's ending")
	require.False(t, Interrupted(endedWithCause, errors.New("sample cause")), "a failure of the work's own that reads like the cause is not the ending")
}
