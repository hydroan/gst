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

// TestStopClosesTheConnectionsTheDrainLeftOpen proves Stop waits for the
// requests in flight only for as long as it may — until its bound passes, or
// until it is told not to wait — and then closes their connections instead
// of leaving them open: a request that outlives the drain is cut off.
func TestStopClosesTheConnectionsTheDrainLeftOpen(t *testing.T) {
	t.Run("once the drain times out", func(t *testing.T) {
		original := drainTimeout
		drainTimeout = 200 * time.Millisecond
		t.Cleanup(func() { drainTimeout = original })
		requested, served := serveARequestThatStaysInFlight(t)

		Stop(context.Background())
		require.Error(t, awaitResult(t, requested), "the request still in flight must have been cut off")
		require.ErrorIs(t, awaitResult(t, served), http.ErrServerClosed)
	})

	t.Run("once the wait is abandoned", func(t *testing.T) {
		requested, served := serveARequestThatStaysInFlight(t)

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
		require.Error(t, awaitResult(t, requested), "the request in flight must have been cut off")
		require.ErrorIs(t, awaitResult(t, served), http.ErrServerClosed)
	})
}

// serveARequestThatStaysInFlight serves, as the package's server, a handler
// that does not finish before the test does, sends it one request and waits
// for the request to reach it. It returns what the request and the server
// end with.
func serveARequestThatStaysInFlight(t *testing.T) (requested, served <-chan error) {
	t.Helper()

	entered, release := make(chan struct{}, 1), make(chan struct{})
	t.Cleanup(func() { close(release) })
	srv := newServer("", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		entered <- struct{}{}
		<-release
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serving := make(chan error, 1)
	go func() { serving <- srv.Serve(listener) }()
	original := server
	server = srv
	t.Cleanup(func() { server = original })

	requesting := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+listener.Addr().String(), nil)
		if err == nil {
			var rsp *http.Response
			if rsp, err = http.DefaultClient.Do(req); err == nil {
				rsp.Body.Close()
			}
		}
		requesting <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached its handler")
	}
	return requesting, serving
}

// awaitResult receives the next result from results, failing the test when
// none comes in time.
func awaitResult(t *testing.T, results <-chan error) error {
	t.Helper()

	select {
	case err := <-results:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("no result came in time")
		return nil
	}
}
