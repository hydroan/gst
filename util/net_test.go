package util_test

import (
	"net"
	"syscall"
	"testing"

	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

func TestGetFdFromConn(t *testing.T) {
	conn, _ := dialLocal(t)
	tcp, ok := conn.(*net.TCPConn)
	require.True(t, ok)

	raw, err := tcp.SyscallConn()
	require.NoError(t, err)
	require.Equal(t, sysfd(t, raw), util.GetFdFromConn(conn))
}

func TestGetFdFromListener(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	tcp, ok := l.(*net.TCPListener)
	require.True(t, ok)

	raw, err := tcp.SyscallConn()
	require.NoError(t, err)
	require.Equal(t, sysfd(t, raw), util.GetFdFromListener(l))
}

// dialLocal opens a TCP connection to a listener on the loopback interface, so
// the tests depend on no network outside the machine.
func dialLocal(t *testing.T) (net.Conn, net.Listener) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	conn, err := net.Dial("tcp", l.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn, l
}

// sysfd returns the file descriptor raw wraps.
func sysfd(t *testing.T, raw syscall.RawConn) int {
	t.Helper()

	var fd int
	require.NoError(t, raw.Control(func(f uintptr) { fd = int(f) }))
	return fd
}
