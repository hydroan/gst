package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// TestGoReportsAFailureWhileSiblingsKeepRunning pins the case the process used
// to hang on: one long-running function fails to start while another keeps
// serving. The failure has to reach the caller at once, not after the sibling
// that is still serving eventually returns.
func TestGoReportsAFailureWhileSiblingsKeepRunning(t *testing.T) {
	errStart := errors.New("sample start failure")
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	in := new(initializer)
	in.RegisterGo(
		func() error { <-release; return nil },
		func() error { return errStart },
	)

	failed := in.Go()
	select {
	case <-failed.Done():
		require.ErrorIs(t, context.Cause(failed), errStart)
	case <-time.After(time.Second):
		t.Fatal("a failed function went unreported while a sibling kept running")
	}
}

// TestGoIsNotCanceledByAFunctionThatReturnsNil covers the listeners that are
// not enabled: they return nil at once, and must not be taken for a failure
// that would end the process while the server beside them keeps serving.
func TestGoIsNotCanceledByAFunctionThatReturnsNil(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	in := new(initializer)
	in.RegisterGo(
		func() error { return nil },
		func() error { <-release; return nil },
	)

	failed := in.Go()
	select {
	case <-failed.Done():
		t.Fatalf("a function returning nil canceled the run: %v", context.Cause(failed))
	case <-time.After(100 * time.Millisecond):
	}
}
