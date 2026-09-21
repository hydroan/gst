package util_test

import (
	"net"
	"testing"
	"time"

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
