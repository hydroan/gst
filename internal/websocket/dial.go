package websocket

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	coderwebsocket "github.com/coder/websocket"
)

// maxRefusalBytes is the most Dial keeps of the answer to a handshake the
// server refuses.
const maxRefusalBytes = 1 << 20

// Dial opens a WebSocket connection to the server at url, an http or https
// address, and returns its client end, which the caller closes. client
// carries the handshake, http.DefaultClient when nil, with header among its
// headers; ctx bounds the handshake alone, and writeTimeout each Send, zero
// for no bound.
//
// A handshake the server refuses fails, and the response returned is the
// server's answer to it, for the caller to read the refusal from: its body is
// the answer, the first maxRefusalBytes of it, read into memory. The response
// of a handshake accepted has no body, the connection being the Conn's.
// Neither is the caller's to close.
func Dial(ctx context.Context, client *http.Client, url string, header http.Header, writeTimeout time.Duration) (*Conn, *http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	// The library keeps the first kilobyte of the answer to a handshake the
	// server refuses, for its own error, and drops the rest: the handshake
	// goes through a transport that keeps the answer whole.
	answer := &refusalKeeper{transport: client.Transport}
	keeping := *client
	keeping.Transport = answer
	ws, rsp, err := coderwebsocket.Dial(ctx, url, &coderwebsocket.DialOptions{HTTPClient: &keeping, HTTPHeader: header})
	if err != nil {
		if rsp != nil && answer.body != nil {
			rsp.Body = io.NopCloser(bytes.NewReader(answer.body))
		}
		return nil, rsp, errors.Wrap(err, "websocket: dial")
	}
	ws.SetReadLimit(maxMessageBytes)

	// The connection outlives the handshake ctx bounds: its own context ends
	// when the server leaves or the caller closes it.
	connCtx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	conn := &Conn{
		ws:           ws,
		writeTimeout: writeTimeout,
		ctx:          connCtx,
		cancel:       cancel,
		messages:     make(chan []byte),
		recvClosed:   make(chan struct{}),
	}
	go conn.read()
	return conn, rsp, nil
}

// refusalKeeper is the transport a handshake goes through: it reads the
// answer of one the server refuses, any answer but the 101 that accepts it,
// into memory, the first maxRefusalBytes of it, and hands the library a
// response reading from there.
type refusalKeeper struct {
	// transport sends the handshake, http.DefaultTransport when nil.
	transport http.RoundTripper
	// body is the answer of a handshake refused, nil until one is.
	body []byte
}

// RoundTrip sends the handshake r and keeps the answer of one refused.
func (k *refusalKeeper) RoundTrip(r *http.Request) (*http.Response, error) {
	transport := k.transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	rsp, err := transport.RoundTrip(r)
	if err != nil || rsp.StatusCode == http.StatusSwitchingProtocols {
		return rsp, err
	}
	body, err := io.ReadAll(io.LimitReader(rsp.Body, maxRefusalBytes))
	_ = rsp.Body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "websocket: read the answer to the handshake")
	}
	k.body = body
	rsp.Body = io.NopCloser(bytes.NewReader(body))
	return rsp, nil
}
