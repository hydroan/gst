package lifecycle_test

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestShutdownHTTPServerWaitsForTheRequestsInFlight proves a request being
// served when the shutdown begins is answered before the server goes, and
// pins the entries of a shutdown that drained.
func TestShutdownHTTPServerWaitsForTheRequestsInFlight(t *testing.T) {
	logs := observeGlobalLogs(t)
	server, address, entered, release := serveHeldRequests(t)
	requested := request(address)
	awaitRequest(t, entered)

	shutDown := make(chan struct{})
	go func() {
		defer close(shutDown)
		lifecycle.ShutdownHTTPServer(context.Background(), server, "sample server")
	}()
	select {
	case <-shutDown:
		t.Fatal("the shutdown returned with a request still in flight")
	case <-time.After(100 * time.Millisecond):
	}

	release()
	select {
	case <-shutDown:
	case <-time.After(5 * time.Second):
		t.Fatal("the shutdown did not return once the request was answered")
	}
	require.NoError(t, <-requested, "the request in flight must have been answered")
	require.Equal(t, []string{
		"sample server shutdown initiated",
		"sample server shutdown completed",
	}, serverMessages(t, logs))
}

// TestShutdownHTTPServerClosesTheConnectionsItsDrainLeftOpen proves a
// shutdown whose context has ended does not wait for the requests in flight:
// their connections are closed, and the entries say so.
func TestShutdownHTTPServerClosesTheConnectionsItsDrainLeftOpen(t *testing.T) {
	logs := observeGlobalLogs(t)
	server, address, entered, _ := serveHeldRequests(t)
	requested := request(address)
	awaitRequest(t, entered)

	ended, end := context.WithCancel(context.Background())
	end()
	begin := time.Now()
	lifecycle.ShutdownHTTPServer(ended, server, "sample server")
	require.Less(t, time.Since(begin), time.Second, "an ended context must not be waited on")

	select {
	case err := <-requested:
		require.Error(t, err, "the request in flight must have been cut off")
	case <-time.After(5 * time.Second):
		t.Fatal("the request in flight was not cut off")
	}
	require.Equal(t, []string{
		"sample server shutdown initiated",
		"sample server closing the connections its drain left open",
	}, serverMessages(t, logs))
}

// serveHeldRequests serves, on a loopback port of its own, a handler that
// signals entered and then holds its request until release is called; the
// test's end releases what is still held.
func serveHeldRequests(t *testing.T) (server *http.Server, address string, entered <-chan struct{}, release func()) {
	t.Helper()

	reached, held := make(chan struct{}, 1), make(chan struct{})
	release = sync.OnceFunc(func() { close(held) })
	t.Cleanup(release)
	server = &http.Server{
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached <- struct{}{}
			<-held
		}),
		ReadHeaderTimeout: time.Second,
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		require.ErrorIs(t, <-served, http.ErrServerClosed)
	})
	return server, listener.Addr().String(), reached, release
}

// request sends one GET to address on a goroutine of its own and returns the
// channel its outcome arrives on.
func request(address string) <-chan error {
	outcome := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+address, nil)
		if err == nil {
			var rsp *http.Response
			if rsp, err = http.DefaultClient.Do(req); err == nil {
				err = rsp.Body.Close()
			}
		}
		outcome <- err
	}()
	return outcome
}

// awaitRequest waits for the handler to have received the request.
func awaitRequest(t *testing.T, entered <-chan struct{}) {
	t.Helper()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached its handler")
	}
}

// observeGlobalLogs routes the global logger, the one the shutdown logs
// through, to an observer for the length of the test; the entries record
// their caller, as the framework's do.
func observeGlobalLogs(t *testing.T) *observer.ObservedLogs {
	t.Helper()

	core, logs := observer.New(zap.InfoLevel)
	t.Cleanup(zap.ReplaceGlobals(zap.New(core, zap.AddCaller())))
	return logs
}

// serverMessages returns the messages of the entries the sample server's
// shutdown logged, in order, having checked the caller each records: this
// file, the code that asked for the shutdown, and not the file of
// ShutdownHTTPServer.
func serverMessages(t *testing.T, logs *observer.ObservedLogs) []string {
	t.Helper()

	var messages []string
	for _, entry := range logs.All() {
		if !strings.HasPrefix(entry.Message, "sample server") {
			continue
		}
		require.Equal(t, "httpserver_test.go", filepath.Base(entry.Caller.File), "the caller of %q", entry.Message)
		messages = append(messages, entry.Message)
	}
	return messages
}
