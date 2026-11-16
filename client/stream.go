package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/websocket"
)

// ServerStream is the client's end of a Stream action whose response is
// streamed, one declaring a Payload and a StreamingResult: Recv returns the
// responses one by one. The caller closes it.
type ServerStream[RSP any] struct {
	conn *streamConn
}

// ServerStream opens the Stream action at path whose response is streamed,
// sends payload as its request, none when nil, and returns the stream of its
// responses.
//
// A stream travels over HTTP on a WebSocket connection, every message one
// JSON document. It is opened the way a request is sent: the handshake is a
// GET on path, with the query parameters of opts, the headers and the
// credentials of the client and the trace of ctx, and one the server refuses
// surfaces as *Error, like the rejection of any request. A stream the server
// ends with an error surfaces as *Error as well, carrying the status and the
// message it was ended with. ctx bounds the whole stream: once it ends the
// connection is closed, and the stream's methods fail with its error.
func (c *Client) ServerStream[RSP any](ctx context.Context, path string, payload any, opts ...RequestOption) (*ServerStream[RSP], error) {
	conn, err := c.dialStream(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	// The request is the one message the client sends. The frame ending its
	// messages follows, so that a stream sent no request starts at once
	// instead of waiting for one.
	if payload != nil {
		err = conn.send(payload)
	}
	if err == nil {
		err = conn.closeSend()
	}
	if err != nil {
		_ = conn.close()
		return nil, err
	}
	return &ServerStream[RSP]{conn: conn}, nil
}

// Recv returns the next response of the stream, io.EOF once the server has
// ended it without an error, and the *Error it was ended with otherwise.
func (s *ServerStream[RSP]) Recv() (*RSP, error) {
	return recv[RSP](s.conn)
}

// Close ends the stream from the client's side and releases its connection.
func (s *ServerStream[RSP]) Close() error {
	return s.conn.close()
}

// ClientStream is the client's end of a Stream action whose request is
// streamed, one declaring a StreamingPayload and a Result: Send sends the
// requests one by one, and CloseAndRecv returns the response. The caller
// closes it.
type ClientStream[REQ, RSP any] struct {
	conn *streamConn
}

// ClientStream opens the Stream action at path whose request is streamed and
// returns the stream to send its requests on, opened and bounded by ctx the
// way ServerStream opens one.
func (c *Client) ClientStream[REQ, RSP any](ctx context.Context, path string, opts ...RequestOption) (*ClientStream[REQ, RSP], error) {
	conn, err := c.dialStream(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	return &ClientStream[REQ, RSP]{conn: conn}, nil
}

// Send sends req, the next request of the stream.
func (s *ClientStream[REQ, RSP]) Send(req *REQ) error {
	return s.conn.send(req)
}

// CloseAndRecv tells the server the client has sent its last request and
// returns the response the server answers the stream with, the zero response
// for an action declaring none, or the *Error the server ended the stream
// with.
func (s *ClientStream[REQ, RSP]) CloseAndRecv() (*RSP, error) {
	if err := s.conn.closeSend(); err != nil {
		return nil, err
	}
	rsp, err := recv[RSP](s.conn)
	if errors.Is(err, io.EOF) {
		// The server ended the stream without a response to send.
		return new(RSP), nil
	}
	return rsp, err
}

// Close ends the stream from the client's side and releases its connection.
func (s *ClientStream[REQ, RSP]) Close() error {
	return s.conn.close()
}

// BidiStream is the client's end of a Stream action streaming both ways, one
// declaring a StreamingPayload and a StreamingResult: Send sends the requests
// and Recv returns the responses, in whatever order the action exchanges
// them. The caller closes it.
type BidiStream[REQ, RSP any] struct {
	conn *streamConn
}

// BidiStream opens the Stream action at path streaming both ways and returns
// its stream, opened and bounded by ctx the way ServerStream opens one.
func (c *Client) BidiStream[REQ, RSP any](ctx context.Context, path string, opts ...RequestOption) (*BidiStream[REQ, RSP], error) {
	conn, err := c.dialStream(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	return &BidiStream[REQ, RSP]{conn: conn}, nil
}

// Send sends req, the next request of the stream.
func (s *BidiStream[REQ, RSP]) Send(req *REQ) error {
	return s.conn.send(req)
}

// Recv returns the next response of the stream, io.EOF once the server has
// ended it without an error, and the *Error it was ended with otherwise.
func (s *BidiStream[REQ, RSP]) Recv() (*RSP, error) {
	return recv[RSP](s.conn)
}

// CloseSend tells the server the client has sent its last request. The
// stream still receives the responses the server sends after it.
func (s *BidiStream[REQ, RSP]) CloseSend() error {
	return s.conn.closeSend()
}

// Close ends the stream from the client's side and releases its connection.
func (s *BidiStream[REQ, RSP]) Close() error {
	return s.conn.close()
}

// streamConn is the WebSocket connection a stream travels on, with the
// context the stream was opened on.
type streamConn struct {
	conn *websocket.Conn
	ctx  context.Context
	// release stops the watch on ctx that closes the connection when it ends.
	release func() bool
}

// dialStream opens the connection of the stream at path: the handshake is the
// GET newRequest builds, so it carries what any request of the client does. A
// handshake the server refuses is answered like the rejection of a request,
// and returns the *Error of the envelope it answered.
func (c *Client) dialStream(ctx context.Context, path string, opts []RequestOption) (*streamConn, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil, opts)
	if err != nil {
		return nil, err
	}
	conn, rsp, err := websocket.Dial(ctx, c.httpClient, req.URL.String(), req.Header, c.timeout)
	if err != nil {
		if rsp != nil && rsp.Body != nil {
			body, readErr := io.ReadAll(rsp.Body)
			if readErr != nil {
				return nil, errors.Wrap(readErr, "failed to read the response body")
			}
			if _, envErr := parseEnvelope(rsp, body); envErr != nil {
				return nil, envErr
			}
		}
		return nil, errors.Wrap(err, "failed to open the stream")
	}
	s := &streamConn{conn: conn, ctx: ctx}
	s.release = context.AfterFunc(ctx, func() { conn.Close(nil) })
	return s, nil
}

// send sends v as one message of the stream.
func (s *streamConn) send(v any) error {
	message, err := json.Marshal(v)
	if err != nil {
		return errors.Wrap(err, "failed to marshal the message")
	}
	return s.ending(s.conn.Send(message))
}

// closeSend sends the frame that ends the client's messages.
func (s *streamConn) closeSend() error {
	return s.ending(s.conn.CloseSend())
}

// close closes the connection, and stops watching the stream's context.
func (s *streamConn) close() error {
	s.release()
	s.conn.Close(nil)
	return nil
}

// ending returns err, what the connection failed with, as the client reports
// the ending of a stream: nil and io.EOF as they are, the error of the
// stream's context once that has ended, which is what closed the connection,
// the *Error of the status and the message the server ended the stream with,
// and any other failure as it is.
func (s *streamConn) ending(err error) error {
	var ended *types.Error
	switch {
	case err == nil, errors.Is(err, io.EOF):
		return err
	case s.ctx.Err() != nil:
		return context.Cause(s.ctx)
	case errors.As(err, &ended):
		return &Error{StatusCode: ended.Status(), Msg: ended.Msg()}
	default:
		return errors.Wrap(err, "the stream failed")
	}
}

// recv returns the next message of the stream s travels on, decoded into a T.
func recv[T any](s *streamConn) (*T, error) {
	message, err := s.conn.Recv()
	if err != nil {
		return nil, s.ending(err)
	}
	out := new(T)
	if err = json.Unmarshal(message, out); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal the message")
	}
	return out, nil
}
