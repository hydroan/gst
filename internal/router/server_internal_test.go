package router

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/sse"
	"github.com/stretchr/testify/require"
)

// TestShutdownEndsOpenStreams proves an open Server-Sent Events stream does
// not hold the server's shutdown. Shutdown waits for every active request and
// cancels none of their contexts, so a stream watching only its request would
// keep the server up until the client left. The stream ends the moment the
// shutdown begins: its handler returns, the client reads the end of the
// stream, and Shutdown returns well within its bound.
func TestShutdownEndsOpenStreams(t *testing.T) {
	returned := make(chan error, 1)
	srv := newServer("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		returned <- sse.Serve(w, r, func(conn *sse.Conn) error {
			<-conn.Context().Done()
			return nil
		})
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String(), nil)
	require.NoError(t, err)
	rsp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()
	require.Equal(t, http.StatusOK, rsp.StatusCode)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	begin := time.Now()
	require.NoError(t, srv.Shutdown(ctx), "an open stream must not hold the shutdown to its bound")
	require.Less(t, time.Since(begin), time.Second)

	select {
	case serveErr := <-returned:
		require.NoError(t, serveErr)
	case <-time.After(time.Second):
		t.Fatal("the stream's handler must have returned")
	}
	_, err = io.ReadAll(rsp.Body)
	require.NoError(t, err, "the client reads the end of the stream")
	require.ErrorIs(t, <-served, http.ErrServerClosed)
}

// TestStopClosesTheConnectionsOnceTheWaitIsAbandoned proves Stop waits for
// the requests in flight only until it is told not to, then closes their
// connections instead of waiting out its bound: a process that must not wait
// on anything gets no drain.
func TestStopClosesTheConnectionsOnceTheWaitIsAbandoned(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	t.Cleanup(func() { close(release) })
	srv := newServer("", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		entered <- struct{}{}
		<-release
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()
	original := server
	server = srv
	t.Cleanup(func() { server = original })

	requested := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+listener.Addr().String(), nil)
		if err == nil {
			var rsp *http.Response
			if rsp, err = http.DefaultClient.Do(req); err == nil {
				rsp.Body.Close()
			}
		}
		requested <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached its handler")
	}

	abandon, abandonWait := context.WithCancelCause(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		Stop(abandon)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop must wait for the request in flight until it is told not to")
	case <-time.After(200 * time.Millisecond):
	}

	abandonWait(errors.New("sample failure"))
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop must return once the wait is abandoned")
	}
	require.Error(t, <-requested, "the request in flight must have been cut off")
	require.ErrorIs(t, <-served, http.ErrServerClosed)
}
