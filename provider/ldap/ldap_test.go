package ldap

import (
	"net"
	"sync"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestCheckConnectionRestoresNilConnection(t *testing.T) {
	// A nil gconn is the state a failed reconnect leaves behind. The heartbeat
	// must keep retrying from that state; giving up there disconnects the
	// provider until the process restarts.
	host, port := startFakeEndpoint(t)
	configureLdap(t, host, port)
	clearConn(t)

	checkConnection()

	mu.RLock()
	defer mu.RUnlock()
	require.NotNil(t, gconn, "heartbeat must rebuild a nil connection instead of giving up")
}

func TestCheckConnectionKeepsNilWhenServerUnreachable(t *testing.T) {
	// Grab a free port and close the listener so the port refuses connections.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.NoError(t, ln.Close())

	configureLdap(t, "127.0.0.1", addr.Port)
	clearConn(t)

	checkConnection()

	mu.RLock()
	defer mu.RUnlock()
	require.Nil(t, gconn, "reconnect against an unreachable server must leave the connection nil")
}

// startFakeEndpoint listens on a local TCP port and keeps accepted connections
// open. DialURL only performs the TCP handshake, so this stands in for an LDAP
// server as long as the test never binds or searches.
func startFakeEndpoint(t *testing.T) (host string, port int) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	// Keep every accepted connection referenced so the runtime finalizer never
	// closes it while the test still holds the client side.
	var acceptedMu sync.Mutex
	var accepted []net.Conn
	t.Cleanup(func() {
		acceptedMu.Lock()
		defer acceptedMu.Unlock()
		for _, conn := range accepted {
			_ = conn.Close()
		}
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			acceptedMu.Lock()
			accepted = append(accepted, conn)
			acceptedMu.Unlock()
		}
	}()

	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return "127.0.0.1", addr.Port
}

// configureLdap points the global LDAP configuration at the given endpoint and
// restores the previous configuration when the test finishes.
func configureLdap(t *testing.T, host string, port int) {
	t.Helper()

	originalConfig := config.App
	config.App = new(config.Config)
	config.App.Ldap.Enabled = true
	config.App.Ldap.Host = host
	config.App.Ldap.Port = port
	t.Cleanup(func() { config.App = originalConfig })
}

// clearConn empties the global connection for the duration of the test and
// closes whatever connection the test run left behind.
func clearConn(t *testing.T) {
	t.Helper()

	mu.Lock()
	originalConn := gconn
	gconn = nil
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		if gconn != nil {
			gconn.Close()
		}
		gconn = originalConn
		mu.Unlock()
	})
}
