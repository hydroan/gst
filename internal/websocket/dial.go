package websocket

import (
	"context"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	coderwebsocket "github.com/coder/websocket"
)

// Dial opens a WebSocket connection to the server at url, an http or https
// address, and returns its client end, which the caller closes. client
// carries the handshake, http.DefaultClient when nil, with header among its
// headers; ctx bounds the handshake alone, and writeTimeout each Send, zero
// for no bound.
//
// A handshake the server refuses fails, and the response returned is the
// server's answer to it, for the caller to read the refusal from: its body is
// what the library keeps of the answer, the first kilobyte, read into memory.
// The response of a handshake accepted has no body, the connection being the
// Conn's. Neither is the caller's to close.
//
//gst:api the client end of the connections Accept serves
func Dial(ctx context.Context, client *http.Client, url string, header http.Header, writeTimeout time.Duration) (*Conn, *http.Response, error) {
	ws, rsp, err := coderwebsocket.Dial(ctx, url, &coderwebsocket.DialOptions{HTTPClient: client, HTTPHeader: header})
	if err != nil {
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
