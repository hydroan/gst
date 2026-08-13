package sse

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
)

// startStreamServer starts a real HTTP server with aggressively short
// per-request deadlines, the environment a streaming connection must survive.
func startStreamServer(t *testing.T, timeout time.Duration, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.ReadTimeout = timeout
	srv.Config.WriteTimeout = timeout
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// getStream issues a plain GET with a bounded context and returns the live
// response.
func getStream(t *testing.T, url string, header http.Header) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	for key, values := range header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rsp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return rsp
}

func TestServe_StreamOutlivesServerDeadlines(t *testing.T) {
	// The server kills any response not finished within 200ms; the stream
	// below runs for ~1.2s and must survive because Serve cleared the
	// deadlines. The short heartbeat also exercises concurrent writes under
	// the race detector.
	srv := startStreamServer(t, 200*time.Millisecond, func(w http.ResponseWriter, r *http.Request) {
		err := Serve(w, r, func(conn *Conn) error {
			for i := 1; i <= 6; i++ {
				select {
				case <-conn.Context().Done():
					return nil
				case <-time.After(200 * time.Millisecond):
				}
				if err := conn.Send(Event{Event: "tick", Data: i}); err != nil {
					return err
				}
			}
			return nil
		}, WithHeartbeatInterval(50*time.Millisecond))
		if err != nil {
			t.Errorf("Serve failed: %v", err)
		}
	})

	rsp := getStream(t, srv.URL, nil)
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rsp.StatusCode)
	}
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatalf("Reading the stream failed: %v", err)
	}
	if got := strings.Count(string(body), "event: tick"); got != 6 {
		t.Errorf("Expected 6 tick events to survive the deadlines, got %d in %q", got, string(body))
	}
}

func TestServe_HeadersArriveBeforeFirstEvent(t *testing.T) {
	release := make(chan struct{})
	srv := startStreamServer(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		_ = Serve(w, r, func(conn *Conn) error {
			select {
			case <-release:
			case <-conn.Context().Done():
			}
			return nil
		})
	})

	// Do returns once the response headers arrived; without the eager header
	// flush this call would hang until the callback returns.
	rsp := getStream(t, srv.URL, nil)
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rsp.StatusCode)
	}
	if got := rsp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %q", got)
	}
	if got := rsp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("Expected X-Accel-Buffering no, got %q", got)
	}
	close(release)
	_, _ = io.Copy(io.Discard, rsp.Body)
}

func TestServe_HeartbeatFramesFlow(t *testing.T) {
	srv := startStreamServer(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		_ = Serve(w, r, func(conn *Conn) error {
			select {
			case <-conn.Context().Done():
			case <-time.After(300 * time.Millisecond):
			}
			return nil
		}, WithHeartbeatInterval(50*time.Millisecond))
	})

	rsp := getStream(t, srv.URL, nil)
	defer rsp.Body.Close()
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatalf("Reading the stream failed: %v", err)
	}
	if got := strings.Count(string(body), ": ping"); got < 2 {
		t.Errorf("Expected at least 2 heartbeat frames on a quiet stream, got %d in %q", got, string(body))
	}
}

func TestServe_ClientDisconnectCancelsContext(t *testing.T) {
	unblocked := make(chan error, 1)
	srv := startStreamServer(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		_ = Serve(w, r, func(conn *Conn) error {
			if err := conn.Send(Event{Data: "first"}); err != nil {
				unblocked <- err
				return err
			}
			select {
			case <-conn.Context().Done():
				unblocked <- nil
			case <-time.After(5 * time.Second):
				unblocked <- errors.New("context was not canceled after the client left")
			}
			return nil
		})
	})

	rsp := getStream(t, srv.URL, nil)
	defer rsp.Body.Close()
	reader := bufio.NewReader(rsp.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("Reading the first event failed: %v", err)
	}
	rsp.Body.Close()

	if err := <-unblocked; err != nil {
		t.Error(err)
	}
}

func TestServe_LastEventID(t *testing.T) {
	got := make(chan string, 1)
	srv := startStreamServer(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		_ = Serve(w, r, func(conn *Conn) error {
			got <- conn.LastEventID()
			return nil
		})
	})

	header := http.Header{}
	header.Set("Last-Event-ID", "42")
	rsp := getStream(t, srv.URL, header)
	defer rsp.Body.Close()
	_, _ = io.Copy(io.Discard, rsp.Body)

	if id := <-got; id != "42" {
		t.Errorf("Expected Last-Event-ID 42, got %q", id)
	}
}

func TestServe_ConnUnusableAfterCallbackReturns(t *testing.T) {
	leaked := make(chan *Conn, 1)
	srv := startStreamServer(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		_ = Serve(w, r, func(conn *Conn) error {
			leaked <- conn
			return nil
		})
	})

	rsp := getStream(t, srv.URL, nil)
	defer rsp.Body.Close()
	_, _ = io.Copy(io.Discard, rsp.Body)

	conn := <-leaked
	if err := conn.Send(Event{Data: "late"}); err == nil {
		t.Error("Expected a send on a finished connection to fail")
	}
}

func TestServe_SendRejectsInvalidEventAndKeepsStreaming(t *testing.T) {
	srv := startStreamServer(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		_ = Serve(w, r, func(conn *Conn) error {
			if err := conn.Send(Event{}); err == nil {
				t.Error("Expected an empty event to be rejected")
			}
			return conn.Send(Event{Data: "still alive"})
		})
	})

	rsp := getStream(t, srv.URL, nil)
	defer rsp.Body.Close()
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatalf("Reading the stream failed: %v", err)
	}
	if !strings.Contains(string(body), "data: still alive") {
		t.Errorf("Expected the connection to stay usable after a rejected event, got %q", string(body))
	}
}

// opaqueWriter hides the wrapped writer's deadline methods by not exposing
// Unwrap, the shape of a middleware wrapper missing its Unwrap method.
type opaqueWriter struct {
	http.ResponseWriter
}

func (w opaqueWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func TestServe_FailsFastWhenDeadlinesAreUnreachable(t *testing.T) {
	served := make(chan error, 1)
	srv := startStreamServer(t, time.Second, func(w http.ResponseWriter, r *http.Request) {
		served <- Serve(opaqueWriter{w}, r, func(*Conn) error {
			t.Error("The callback must not run when streaming setup fails")
			return nil
		})
		w.WriteHeader(http.StatusInternalServerError)
	})

	rsp := getStream(t, srv.URL, nil)
	defer rsp.Body.Close()
	_, _ = io.Copy(io.Discard, rsp.Body)

	err := <-served
	if err == nil {
		t.Fatal("Expected Serve to fail on a writer chain without Unwrap")
	}
	if !strings.Contains(err.Error(), "Unwrap") {
		t.Errorf("Expected the error to point at the missing Unwrap, got %q", err.Error())
	}
	if rsp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected the handler to still control the response, got status %d", rsp.StatusCode)
	}
}

func TestServe_RejectsBadArguments(t *testing.T) {
	if err := Serve(nil, nil, func(*Conn) error { return nil }); err == nil {
		t.Error("Expected Serve to reject nil writer and request")
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	if err := Serve(recorder, req, nil); err == nil {
		t.Error("Expected Serve to reject a nil callback")
	}
	if err := Serve(recorder, req, func(*Conn) error { return nil }, WithHeartbeatInterval(0)); err == nil {
		t.Error("Expected Serve to reject a non-positive heartbeat interval")
	}
}
