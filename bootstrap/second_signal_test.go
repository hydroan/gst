package bootstrap

import (
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// secondSignalHelper marks the child process that runs Run for the test of a
// shutdown hurried along; the child runs that test alone.
const secondSignalHelper = "GST_BOOTSTRAP_SECOND_SIGNAL_HELPER"

// TestRunDropsItsWaitsOnASecondSignal proves what a second termination
// signal is for: the first began a graceful shutdown and the process is
// holding its drain window open for a load balancer, and the operator has
// decided not to wait it out. The second signal drops that wait and every
// wait left after it, and because being hurried along is not a failure Run
// still returns nil, so the process exits the way a clean shutdown does.
// Run is single-shot and its cleanup unwinds the process, so the run happens
// in a child process: the test binary runs itself again with this test
// selected and the helper marked.
func TestRunDropsItsWaitsOnASecondSignal(t *testing.T) {
	if os.Getenv(secondSignalHelper) == "1" {
		runHurriedAlongBySecondSignal(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunDropsItsWaitsOnASecondSignal$", "-test.v")
	cmd.Env = append(os.Environ(), secondSignalHelper+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "the child process must pass:\n%s", out)
	require.Contains(t, string(out), "--- PASS: TestRunDropsItsWaitsOnASecondSignal", "the child must have run the helper:\n%s", out)
}

// runHurriedAlongBySecondSignal is the child's half: it brings the process up
// with a drain window no test would sit through, sends the process a
// termination signal, waits until the drain is under way, and sends a second
// one.
func runHurriedAlongBySecondSignal(t *testing.T) {
	t.Helper()
	bootstrapProcess(t)
	config.App.Server.ShutdownDelay = time.Minute

	done := make(chan error, 1)
	go func() { done <- Run() }()
	require.Eventually(t, func() bool { return readyz() == http.StatusOK },
		10*time.Second, 20*time.Millisecond, "the server never came up")

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	// Readiness goes down as the shutdown begins, and the drain window the
	// second signal is about opens right after: waiting for the probe is
	// what keeps the second signal from arriving before the first is answered.
	require.Eventually(t, func() bool { return readyz() != http.StatusOK },
		10*time.Second, 20*time.Millisecond, "the shutdown never began")
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	select {
	case err := <-done:
		require.NoError(t, err, "a shutdown hurried along is still a clean one")
	case <-time.After(30 * time.Second):
		t.Fatal("a second signal must drop the drain window, not wait it out")
	}
}
