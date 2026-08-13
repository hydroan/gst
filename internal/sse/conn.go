package sse

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
)

// DefaultHeartbeatInterval is the pause between automatic keep-alive comment
// frames. It stays well below the idle timeouts commonly configured on
// reverse proxies (nginx defaults to 60s), so an event-quiet connection is
// never mistaken for a dead one.
const DefaultHeartbeatInterval = 15 * time.Second

// heartbeatComment is the payload of automatic keep-alive frames. Comment
// frames are invisible to EventSource clients, so the text only shows up on
// the wire.
const heartbeatComment = "ping"

// Option configures Serve.
type Option func(*options)

type options struct {
	heartbeatInterval time.Duration
}

// WithHeartbeatInterval overrides DefaultHeartbeatInterval. The interval must
// be positive; heartbeats cannot be disabled, because a connection without
// keep-alive traffic silently dies on idle proxy timeouts and undetected
// client disconnects.
func WithHeartbeatInterval(interval time.Duration) Option {
	return func(o *options) {
		o.heartbeatInterval = interval
	}
}

// Conn is one live SSE connection. It is handed to the Serve callback and is
// only valid until the callback returns. All methods are safe for concurrent
// use; event writes and heartbeat frames are serialized internally.
type Conn struct {
	mu      sync.Mutex
	w       http.ResponseWriter
	flusher http.Flusher
	closed  bool

	ctx         context.Context
	lastEventID string
}

// Context returns the request context. It is canceled when the client
// disconnects, the server shuts down, or the surrounding request is aborted;
// a Serve callback that waits for work must select on it.
func (c *Conn) Context() context.Context {
	if c == nil {
		return context.Background()
	}
	return c.ctx
}

// LastEventID returns the Last-Event-ID request header, which carries the ID
// of the last event the client received before reconnecting. It is empty on
// a fresh connection. Resuming from it is the caller's decision; the
// framework does not replay events.
func (c *Conn) LastEventID() string {
	if c == nil {
		return ""
	}
	return c.lastEventID
}

// Send validates the event, writes it as one frame and flushes it to the
// client. Invalid events are rejected without touching the connection. A
// failed or canceled connection rejects all further sends.
func (c *Conn) Send(event Event) error {
	if c == nil {
		return errors.New("sse: connection is nil")
	}
	if err := event.validate(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("sse: connection is closed")
	}
	if err := c.ctx.Err(); err != nil {
		c.closed = true
		return errors.Wrap(err, "sse: connection context is done")
	}
	if err := Encode(c.w, event); err != nil {
		// validate already passed, so this is a write failure: the client is
		// gone or the connection broke. Reject further sends.
		c.closed = true
		return err
	}
	c.flusher.Flush()
	return nil
}

// markClosed rejects all further sends. Serve calls it after the callback
// returned and the heartbeat stopped; a Conn leaked past its callback then
// errors instead of writing into a finished response.
func (c *Conn) markClosed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

// Serve turns the response into a Server-Sent Events stream and runs fn with
// the live connection. It owns the connection lifecycle:
//
//   - clears the server's per-request read/write deadlines, so a global
//     WriteTimeout does not kill the long-lived stream;
//   - writes the SSE response headers and flushes them, so the client's
//     EventSource fires its open event before the first business event;
//   - sends keep-alive comment frames on a fixed interval until fn returns;
//   - rejects sends on the connection once fn returned, so a leaked Conn can
//     never write into a finished response.
//
// fn blocks until the stream is over; returning ends the stream. A callback
// that waits for events must select on conn.Context().Done() to notice the
// client disconnecting. The returned error is fn's error, or the setup
// failure that prevented streaming (reported before anything was written, so
// the caller can still answer with a regular error response).
func Serve(w http.ResponseWriter, r *http.Request, fn func(conn *Conn) error, opts ...Option) error {
	if w == nil || r == nil {
		return errors.New("sse: response writer and request cannot be nil")
	}
	if fn == nil {
		return errors.New("sse: serve callback cannot be nil")
	}

	o := options{heartbeatInterval: DefaultHeartbeatInterval}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.heartbeatInterval <= 0 {
		return errors.Newf("sse: heartbeat interval %s is not positive", o.heartbeatInterval)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.Newf("sse: response writer %T does not implement http.Flusher", w)
	}

	// Streaming responses outlive the server's per-request deadlines: with the
	// default 15s WriteTimeout the kernel would tear the connection down mid
	// stream. Clearing the deadlines requires every response writer wrapper in
	// the middleware chain to expose Unwrap; failing fast here is what keeps
	// that contract honest.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		return errors.Wrap(err, "sse: failed to clear the write deadline (a response writer wrapper is missing Unwrap)")
	}
	if err := rc.SetReadDeadline(time.Time{}); err != nil {
		return errors.Wrap(err, "sse: failed to clear the read deadline (a response writer wrapper is missing Unwrap)")
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	conn := &Conn{
		w:           w,
		flusher:     flusher,
		ctx:         r.Context(),
		lastEventID: r.Header.Get("Last-Event-ID"),
	}
	// Deferred in reverse order: stop the heartbeat first, mark closed second,
	// so no frame is ever written after Serve returned.
	defer conn.markClosed()

	stop := make(chan struct{})
	done := make(chan struct{})
	go heartbeat(conn, o.heartbeatInterval, stop, done)
	defer func() {
		close(stop)
		<-done
	}()

	return fn(conn)
}

// heartbeat sends keep-alive comment frames on a fixed interval until the
// stream stops, the connection dies, or a send fails.
func heartbeat(conn *Conn, interval time.Duration, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-conn.ctx.Done():
			return
		case <-ticker.C:
			if err := conn.Send(Event{Comment: heartbeatComment}); err != nil {
				return
			}
		}
	}
}
