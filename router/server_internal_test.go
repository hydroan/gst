package router

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

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
