package util_test

import (
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

// TestTcping reports a port as reachable while something listens on it and
// unreachable once the listener closes. The listener is the test's own, so the
// outcome depends on no network outside the machine.
func TestTcping(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	require.True(t, util.Tcping("127.0.0.1", addr.Port, time.Second))

	require.NoError(t, listener.Close())
	require.False(t, util.Tcping("127.0.0.1", addr.Port, time.Second))
}

// runOrDieHelper marks the child process that runs RunOrDie for
// TestRunOrDieReportsTheCauseWithItsStack.
const runOrDieHelper = "GST_UTIL_RUN_OR_DIE_HELPER"

// TestRunOrDieReportsTheCauseWithItsStack pins the report of a failed start:
// the line on stdout names the function and the cause chain, the stack the
// cause was created with follows it, so a terminal shows where the failure
// began, and the process exits with 1. RunOrDie exits the process, so the run
// happens in a child process: the test binary runs itself again with this
// test selected and the helper marked.
func TestRunOrDieReportsTheCauseWithItsStack(t *testing.T) {
	if os.Getenv(runOrDieHelper) == "1" {
		// The start that fails, with a cause created where a listener would
		// be opened.
		util.RunOrDie(func() error { return errors.Wrap(errors.New("listen failed"), "start the listener") })
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunOrDieReportsTheCauseWithItsStack$")
	cmd.Env = append(os.Environ(), runOrDieHelper+"=1")
	out, err := cmd.CombinedOutput()

	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit, "the child must exit on the error:\n%s", out)
	require.Equal(t, 1, exit.ExitCode())
	report := string(out)
	require.Contains(t, report, "TestRunOrDieReportsTheCauseWithItsStack.func1 error: start the listener: listen failed")
	require.Contains(t, report, "attached stack trace", "the report carries the stack of the cause")
	require.Contains(t, report, "TestRunOrDieReportsTheCauseWithItsStack.func1\n", "the stack names where the cause was created")
}
