package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	coderwebsocket "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// TestHeartbeatPingsAnIdleConnection proves an accepted connection carrying
// no message is not left silent: the server pings its client on every beat
// of the heartbeat, played out here in milliseconds.
func TestHeartbeatPingsAnIdleConnection(t *testing.T) {
	// A beat is also the time a ping has to be written in, and a ping still
	// being written when it runs out closes the connection: the beat leaves
	// room for a test binary a busy machine keeps waiting.
	beatEvery(t, 100*time.Millisecond)

	held := make(chan struct{})
	// The handler is let go whatever the test ends on: a connection it
	// leaves open is one the Drain of every later test waits for.
	release := sync.OnceFunc(func() { close(held) })
	t.Cleanup(release)
	served := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(served)
		conn, err := Accept(w, r, nil, time.Second)
		if err != nil {
			return
		}
		defer conn.Close(nil)
		<-held
	}))
	t.Cleanup(srv.Close)

	var pings atomic.Int32
	client, _, err := coderwebsocket.Dial(t.Context(), srv.URL, &coderwebsocket.DialOptions{
		OnPingReceived: func(context.Context, []byte) bool {
			pings.Add(1)
			return true
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.CloseNow() })
	// A ping is received by a client that reads.
	go func() { _, _, _ = client.Read(context.WithoutCancel(t.Context())) }()

	require.Eventually(t, func() bool { return pings.Load() >= 3 }, 5*time.Second, 10*time.Millisecond)
	release()
	<-served
}

// TestHeartbeatNoticesAClientGoneBehindAMessageUnread proves an accepted
// connection notices its client leaving while a message the client sent still
// waits for Recv, when nothing reads the connection: the ping that can no
// longer be sent ends the context, canceled.
func TestHeartbeatNoticesAClientGoneBehindAMessageUnread(t *testing.T) {
	beatEvery(t, 50*time.Millisecond)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	ended := make(chan error, 1)
	served := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(served)
		conn, err := Accept(w, r, nil, time.Second)
		if err != nil {
			ended <- err
			return
		}
		defer conn.Close(nil)
		select {
		case <-conn.Context().Done():
			ended <- context.Cause(conn.Context())
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)

	client, _, err := coderwebsocket.Dial(t.Context(), srv.URL, nil)
	require.NoError(t, err)
	require.NoError(t, client.Write(t.Context(), coderwebsocket.MessageText, []byte(`{"n":1}`)))
	require.NoError(t, client.CloseNow())

	select {
	case err := <-ended:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("the context of a connection whose client is gone must end")
	}
	<-served
}

// beatEvery has the connections accepted during the test ping every interval,
// a heartbeat played out in milliseconds.
func beatEvery(t *testing.T, interval time.Duration) {
	t.Helper()

	original := heartbeatInterval
	heartbeatInterval = interval
	t.Cleanup(func() { heartbeatInterval = original })
}
