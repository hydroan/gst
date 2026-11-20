package testcontainer

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/wait"
)

// readyDeadlineTarget is a container no strategy finds running, which ends a
// wait for it at once; it records how long the wait asking for its state had
// left.
type readyDeadlineTarget struct {
	wait.StrategyTarget

	left time.Duration
}

func (t *readyDeadlineTarget) State(ctx context.Context) (*container.State, error) {
	if deadline, ok := ctx.Deadline(); ok {
		t.left = time.Until(deadline)
	}
	return nil, errors.New("not running")
}

func (t *readyDeadlineTarget) Logs(context.Context) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// TestExtendReadyWaitGivesTheWholeWaitTheReadyTimeout pins that the wait for
// a container is given readyTimeout all the way down: the startup timeout of
// each kind of strategy the modules started here wait with, one that came
// with a shorter timeout of its own among them, and the deadline of every
// wait made of several, which a strategy inside it shows by the time it is
// left with; that no wait at all is left alone; and that a strategy of a
// kind not known here is an error rather than a wait left at its minute.
func TestExtendReadyWaitGivesTheWholeWaitTheReadyTimeout(t *testing.T) {
	line := wait.ForLog("ready")
	health := wait.ForHTTP("/")
	port := wait.ForListeningPort("6379/tcp").WithStartupTimeout(10 * time.Second)
	ready := wait.ForAll(wait.ForAll(line).WithDeadline(time.Minute), health, port).WithDeadline(time.Minute)

	require.NoError(t, extendReadyWait(ready))

	for _, strategy := range []wait.StrategyTimeout{line, health, port} {
		require.NotNil(t, strategy.Timeout(), "%T", strategy)
		require.Equal(t, readyTimeout, *strategy.Timeout(), "%T", strategy)
	}
	target := &readyDeadlineTarget{}
	require.Error(t, ready.WaitUntilReady(t.Context(), target))
	require.Greater(t, target.left, time.Minute, "the deadlines of both waits and the timeout of the strategy bound the time it is left with, the minute each came with among them")

	require.NoError(t, extendReadyWait(nil))
	require.ErrorContains(t, extendReadyWait(wait.ForExec([]string{"true"})), "*wait.ExecStrategy")
}
