package websocket

import (
	"context"
	"maps"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	coderwebsocket "github.com/coder/websocket"
	"github.com/hydroan/gst/internal/streamctx"
	"github.com/hydroan/gst/internal/types"
)

// Accept upgrades the request to a WebSocket connection and returns its
// server end, which the caller closes when its stream is over. origins lists
// the patterns of the origins admitted beside the host the request is sent
// to, as the OriginPatterns of the library reads them, and writeTimeout
// bounds each Send, zero for no bound.
//
// A handshake refused, a request that is none, an origin not admitted, a
// response that cannot be taken over, writes nothing: Accept returns the
// refusal as a service error carrying the status and the text the library
// refuses with, for the caller to answer the way it answers every refusal,
// and sets the headers the protocol answers such a request with.
func Accept(w http.ResponseWriter, r *http.Request, origins []string, writeTimeout time.Duration) (*Conn, error) {
	held := &handshake{ResponseWriter: w, header: make(http.Header)}
	ws, err := coderwebsocket.Accept(held, r, &coderwebsocket.AcceptOptions{OriginPatterns: origins})
	if err != nil {
		// What the protocol answers the request with goes with the answer
		// the caller writes; the content type of the library's text does
		// not.
		for name, values := range held.header {
			if name != "Content-Type" && name != "X-Content-Type-Options" {
				w.Header()[name] = values
			}
		}
		return nil, types.NewErrorWithCause(held.status, strings.TrimSpace(held.text.String()), err)
	}
	ws.SetReadLimit(maxMessageBytes)

	stream, stopWatch := streamctx.New(r.Context())
	ctx, cancel := context.WithCancelCause(stream)
	conn := &Conn{
		ws:           ws,
		accepted:     true,
		writeTimeout: writeTimeout,
		ctx:          ctx,
		cancel:       cancel,
		stopWatch:    stopWatch,
		messages:     make(chan []byte),
		recvClosed:   make(chan struct{}),
	}
	hold(conn)
	go conn.read()
	go conn.beat(heartbeatInterval)
	return conn, nil
}

// ValidateOrigins reports the first of origins the library could not read as
// a pattern, the way it reads the OriginPatterns Accept hands it: lower-cased,
// by path.Match. The library has no check of its own to call: it reads a
// pattern only once a handshake from another origin reaches it, and refuses
// that handshake over a malformed one, so a server validates the patterns it
// is configured with before it serves.
func ValidateOrigins(origins []string) error {
	for _, pattern := range origins {
		if _, err := path.Match(strings.ToLower(pattern), ""); err != nil {
			return errors.Wrapf(err, "malformed origin pattern %q", pattern)
		}
	}
	return nil
}

// handshake stands between the library and the response during the upgrade.
// The library answers a handshake it refuses itself, in plain text, where
// every refusal of the framework is answered by the caller in one form: the
// status and the text of the refusal are held back here, and the 101 of a
// handshake accepted goes through with its headers.
type handshake struct {
	http.ResponseWriter

	// header is what the library sets, the response's only once the
	// handshake is accepted.
	header http.Header
	// status and text are the refusal the library answered with.
	status int
	text   strings.Builder
}

// Header returns the headers the library sets, apart from the response's.
func (h *handshake) Header() http.Header {
	return h.header
}

// WriteHeader passes the 101 of an accepted handshake on, with the headers
// the library set, and holds any other status back.
func (h *handshake) WriteHeader(status int) {
	if status != http.StatusSwitchingProtocols {
		h.status = status
		return
	}
	maps.Copy(h.ResponseWriter.Header(), h.header)
	h.ResponseWriter.WriteHeader(status)
}

// Write holds back the text of a refusal, the only body the library writes.
func (h *handshake) Write(text []byte) (int, error) {
	return h.text.Write(text)
}

// WriteHeaderNow has a response that buffers its header, gin's, send the 101
// before the library takes the connection over.
func (h *handshake) WriteHeaderNow() {
	if w, ok := h.ResponseWriter.(interface{ WriteHeaderNow() }); ok {
		w.WriteHeaderNow()
	}
}

// Unwrap returns the response the library takes the connection over from.
func (h *handshake) Unwrap() http.ResponseWriter {
	return h.ResponseWriter
}

// open tracks the connections Accept returned until each is closed, for
// Drain to wait on.
var open struct {
	sync.Mutex

	conns map[*Conn]struct{}
	// drained is closed once conns runs empty while a Drain waits on it.
	drained chan struct{}
}

// hold records c as open.
func hold(c *Conn) {
	open.Lock()
	defer open.Unlock()
	if open.conns == nil {
		open.conns = make(map[*Conn]struct{})
	}
	open.conns[c] = struct{}{}
}

// release records c as closed, and tells a Drain waiting once none is open.
func release(c *Conn) {
	open.Lock()
	defer open.Unlock()
	delete(open.conns, c)
	if len(open.conns) == 0 && open.drained != nil {
		close(open.drained)
		open.drained = nil
	}
}

// Drain waits for every connection Accept returned to be closed, for as long
// as ctx lasts, then cuts the ones still open, without the closing handshake,
// and returns how many it cut. A server calls it once it has begun to shut
// down: the connections' contexts have ended by then, and each stream closes
// its own with the status of its ending; http.Server.Shutdown returns without
// waiting for a connection taken over from it, and closes none.
func Drain(ctx context.Context) int {
	open.Lock()
	if len(open.conns) == 0 {
		open.Unlock()
		return 0
	}
	if open.drained == nil {
		open.drained = make(chan struct{})
	}
	drained := open.drained
	open.Unlock()

	select {
	case <-drained:
		return 0
	case <-ctx.Done():
	}

	open.Lock()
	left := make([]*Conn, 0, len(open.conns))
	for c := range open.conns {
		left = append(left, c)
	}
	open.Unlock()
	for _, c := range left {
		_ = c.ws.CloseNow()
	}
	return len(left)
}
