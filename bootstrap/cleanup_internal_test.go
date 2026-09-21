package bootstrap

import (
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/stretchr/testify/require"
)

// TestCleanupsRunSeriallyInReverseOrder proves cleanups run one at a time in
// reverse registration order, like a defer stack: what was brought up last
// (the HTTP listener draining in-flight requests) is torn down first, and
// what everything else depends on (connections, log writers) is torn down
// last. Concurrent teardown would rip the infrastructure out from under the
// drain.
func TestCleanupsRunSeriallyInReverseOrder(t *testing.T) {
	originalCleanups := cleanups
	cleanups = nil
	t.Cleanup(func() { cleanups = originalCleanups })

	var order []int
	var active, maxActive int32
	for i := range 3 {
		registerCleanup(func() {
			now := atomic.AddInt32(&active, 1)
			if now > atomic.LoadInt32(&maxActive) {
				atomic.StoreInt32(&maxActive, now)
			}
			// Long enough that concurrently started cleanups would overlap.
			time.Sleep(20 * time.Millisecond)
			order = append(order, i)
			atomic.AddInt32(&active, -1)
		})
	}

	runCleanups()

	require.Equal(t, []int{2, 1, 0}, order, "cleanups must run in reverse registration order")
	require.EqualValues(t, 1, maxActive, "cleanups must never overlap")
}

// hangingTeardownHelper marks the child process that runs Run for
// TestRunLeavesAHangingTeardownBehindOnceTheProcessFailsNow.
const hangingTeardownHelper = "GST_BOOTSTRAP_HANGING_TEARDOWN_HELPER"

// TestRunLeavesAHangingTeardownBehindOnceTheProcessFailsNow proves the bound
// on the teardown of a process that fails now: a cleanup that never returns
// — a provider that will not close — is left behind once failNowTimeout has
// passed, and Run returns the failure, so the process exits. Run is
// single-shot and the failure is process-wide, so the run happens in a child
// process: the test binary runs itself again with this test selected and the
// helper marked.
func TestRunLeavesAHangingTeardownBehindOnceTheProcessFailsNow(t *testing.T) {
	if os.Getenv(hangingTeardownHelper) == "1" {
		runFailingNowWithAHangingTeardown(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunLeavesAHangingTeardownBehindOnceTheProcessFailsNow$", "-test.v")
	cmd.Env = append(os.Environ(), hangingTeardownHelper+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "the child process must pass:\n%s", out)
	require.Contains(t, string(out), "--- PASS: TestRunLeavesAHangingTeardownBehindOnceTheProcessFailsNow", "the child must have run the helper:\n%s", out)
}

// runFailingNowWithAHangingTeardown is the child's half: it brings the process
// up with a cleanup that never returns, fails the process now, and checks
// that Run ends within the bound all the same.
func runFailingNowWithAHangingTeardown(t *testing.T) {
	t.Helper()
	bootstrapProcess(t)
	failNowTimeout = 300 * time.Millisecond

	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	registerCleanup(func() { <-hang })

	done := make(chan error, 1)
	go func() { done <- Run() }()
	require.Eventually(t, func() bool { return readyz() == http.StatusOK },
		10*time.Second, 20*time.Millisecond, "the server never came up")

	errFailure := errors.New("sample failure the shutdown must not wait on")
	failed := time.Now()
	lifecycle.FailNow(errFailure)
	select {
	case err := <-done:
		require.ErrorIs(t, err, errFailure)
		require.Less(t, time.Since(failed), 3*time.Second, "Run must leave the hanging cleanup behind once the bound passed")
	case <-time.After(20 * time.Second):
		t.Fatal("Run waited on a cleanup that never returns although the process failed now")
	}
}
