package bootstrap

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/router"
	"github.com/stretchr/testify/require"
)

// startSignalHelper marks the child process that runs Run for
// TestRunStopsCleanlyOnASignalDuringTheStart.
const startSignalHelper = "GST_BOOTSTRAP_START_SIGNAL_HELPER"

// TestRunStopsCleanlyOnASignalDuringTheStart proves a termination signal
// that arrives while the routes-ready hooks run ends Run with nothing to
// report, before any component starts. Run is single-shot and its cleanup
// unwinds the process, so the run happens in a child process: the test
// binary runs itself again with this test selected and the helper marked.
func TestRunStopsCleanlyOnASignalDuringTheStart(t *testing.T) {
	if os.Getenv(startSignalHelper) == "1" {
		runSignaledDuringTheStart(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunStopsCleanlyOnASignalDuringTheStart$", "-test.v")
	cmd.Env = append(os.Environ(), startSignalHelper+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "the child process must pass:\n%s", out)
	require.Contains(t, string(out), "--- PASS: TestRunStopsCleanlyOnASignalDuringTheStart", "the child must have run the helper:\n%s", out)
}

// runSignaledDuringTheStart is the child's half: it brings the process up,
// sends itself the signal from inside a hook, and checks how Run ends.
func runSignaledDuringTheStart(t *testing.T) {
	t.Helper()
	bootstrapProcess(t)

	hooked, started := false, false
	lifecycle.Register(lifecycle.Component{
		Name:  "sample-start-probe",
		Stage: lifecycle.StageComponent,
		Start: func(context.Context) error {
			started = true
			return nil
		},
	})
	router.OnRoutesReady(func(ctx context.Context, _ map[string][]string) error {
		hooked = true
		require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
		<-ctx.Done()
		return ctx.Err()
	})

	require.NoError(t, Run(), "a signal during the start is nothing to report")
	require.True(t, hooked, "the hook must have run for the signal to arrive during it")
	require.False(t, started, "no component starts once the process was told to stop")
}
