package router

import (
	"context"
	"fmt"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// withRoutesReadyHooks gives the test a hook registry of its own and restores
// the process's afterwards.
func withRoutesReadyHooks(t *testing.T) {
	t.Helper()

	routesReadyMu.Lock()
	saved := routesReadyHooks
	routesReadyHooks = nil
	routesReadyMu.Unlock()
	t.Cleanup(func() {
		routesReadyMu.Lock()
		routesReadyHooks = saved
		routesReadyMu.Unlock()
	})
}

// TestRoutesReadyHooksRunOnTheContextAndStopWithIt proves the hooks run on
// the context they are handed and a stop reaches them before as well as
// during one: a hook sees the very context, and once it has ended no further
// hook starts and the ending is what comes back.
func TestRoutesReadyHooksRunOnTheContextAndStopWithIt(t *testing.T) {
	withRoutesReadyHooks(t)

	type key struct{}
	ctx, stop := context.WithCancelCause(context.WithValue(context.Background(), key{}, "sample"))
	var ran []string
	OnRoutesReady(func(ctx context.Context, _ map[string][]string) error {
		ran = append(ran, fmt.Sprint(ctx.Value(key{})))
		stop(errors.New("sample stop"))
		return nil
	})
	OnRoutesReady(func(context.Context, map[string][]string) error {
		ran = append(ran, "second")
		return nil
	})

	err := RunRoutesReadyHooks(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"sample"}, ran, "no hook starts once the context has ended")
}
