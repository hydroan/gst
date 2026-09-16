package debugpprof

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStopClosesTheConnectionsOnceTheWaitIsAbandoned proves Stop gives the
// requests in flight no drain once it is told not to wait: their
// connections are closed at once, for a process that must not wait on
// anything.
func TestStopClosesTheConnectionsOnceTheWaitIsAbandoned(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	t.Cleanup(func() { close(release) })
	srv := &http.Server{
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			entered <- struct{}{}
			<-release
		}),
		ReadHeaderTimeout: time.Second,
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()
	server = srv
	t.Cleanup(func() { server = nil })

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

	abandoned, abandon := context.WithCancel(context.Background())
	abandon()
	begin := time.Now()
	Stop(abandoned)
	require.Less(t, time.Since(begin), time.Second, "Stop must not wait for the request once told not to")
	select {
	case err := <-requested:
		require.Error(t, err, "the request in flight must have been cut off")
	case <-time.After(5 * time.Second):
		t.Fatal("the request in flight was not cut off")
	}
	require.ErrorIs(t, <-served, http.ErrServerClosed)
}
