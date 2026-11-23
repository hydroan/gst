// Package websocket serves and dials the WebSocket connections a stream
// travels on over HTTP: every message one text frame holding a JSON document,
// the client's last one followed by a frame holding null.
//
// github.com/coder/websocket speaks the protocol. This package adds what a
// stream needs around it, and keeps the library out of every other package:
// a reader that ends the connection's context when the peer leaves, which the
// context of a request no longer does once its connection is taken over from
// the HTTP server; the server's shutdown ending that context too (see
// streamctx); a bound on each send and on the size of a message; a
// heartbeat; the close statuses the endings of a stream are told apart by;
// and Drain, the wait a shutdown gives the connections accepted, which
// http.Server.Shutdown neither waits for nor closes.
package websocket

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	coderwebsocket "github.com/coder/websocket"
	"github.com/hydroan/gst/internal/i18n"
	"github.com/hydroan/gst/internal/sse"
	"github.com/hydroan/gst/internal/streamctx"
	"github.com/hydroan/gst/internal/types"
)

// maxMessageBytes is the largest message a connection reads. A larger one
// closes the connection with StatusMessageTooBig.
const maxMessageBytes = 4 << 20

// maxCloseReasonBytes is the longest reason a close frame carries: a control
// frame holds 125 bytes, two of them the status.
const maxCloseReasonBytes = 123

// errorStatusBase is what the close status of a stream ending in an error adds
// to the HTTP status the error answers with: a service error of status 404
// closes the connection with 4404, in the range the protocol leaves to
// applications, where its own statuses name no ending a client can act on.
const errorStatusBase coderwebsocket.StatusCode = 4000

// endOfMessages is what the frame a client sends after its last message
// holds.
const endOfMessages = "null"

// The reasons of the closes a connection decides on what its peer sent.
const (
	binaryReason    = "binary messages are not supported"
	afterLastReason = "a message arrived after the last"
)

// heartbeatInterval is the pause between the pings an accepted connection
// sends, the pause a Server-Sent Events stream keeps between its keep-alive
// frames. A variable so a test can play it out in milliseconds.
var heartbeatInterval = sse.DefaultHeartbeatInterval

// The endings of a connection its own side decides, each the cause its
// context ends with.
var (
	errClosed           = errors.New("websocket: the connection is closed")
	errBinaryMessage    = errors.New("websocket: the peer sent a binary message")
	errMessageAfterLast = errors.New("websocket: the client sent a message after its last")
)

// Conn is one WebSocket connection carrying JSON messages: the server end
// Accept returns, or the client end Dial returns. Its methods are safe for
// concurrent use, Recv by one goroutine at a time.
type Conn struct {
	ws *coderwebsocket.Conn
	// accepted tells the server end from the client end.
	accepted     bool
	writeTimeout time.Duration

	ctx    context.Context
	cancel context.CancelCauseFunc
	// stopWatch releases the watch an accepted connection keeps on the
	// shutdown of its server.
	stopWatch func()

	// messages hands Recv what read reads. read closes it once the peer has
	// no more to send, having set ended to what Recv answers from then on.
	messages chan []byte
	ended    error
	// recvClosed is closed by CloseRecv: Recv hands out no more messages.
	recvClosed    chan struct{}
	closeRecvOnce sync.Once

	closeOnce sync.Once
	// status is the status the connection closed with, the first recorded:
	// the one it closed with itself, or the one its peer closed with.
	status atomic.Int64
}

// Context returns the connection's context. It ends when the peer leaves and
// when the connection is closed; that of an accepted connection ends with its
// request's as well, and the moment the server the request arrived on begins
// to shut down, with streamctx.ErrServerShutdown as its cause.
func (c *Conn) Context() context.Context {
	return c.ctx
}

// Recv returns the next message the peer sent. An accepted connection answers
// io.EOF once the client has sent its last message, whatever ends the
// connection after it. A dialed one answers io.EOF once the server has closed
// the connection without an error, and the error the server closed it with
// otherwise (see closeError). Once the connection's context has ended, Recv
// fails with its cause, and once CloseRecv has been called it answers io.EOF.
func (c *Conn) Recv() ([]byte, error) {
	select {
	case <-c.recvClosed:
		return nil, io.EOF
	default:
	}
	select {
	case message, ok := <-c.messages:
		if !ok {
			return nil, c.ended
		}
		return message, nil
	case <-c.recvClosed:
		return nil, io.EOF
	case <-c.ctx.Done():
		// A client that is through is through, whatever ended the connection
		// after it: the end of its messages, once read has seen it, is the
		// answer over the cause the context ended with.
		select {
		case message, ok := <-c.messages:
			if !ok {
				return nil, c.ended
			}
			return message, nil
		default:
			return nil, context.Cause(c.ctx)
		}
	}
}

// Send sends message, a JSON document, as one frame. It waits for the peer
// no longer than the connection's write timeout: a peer that does not read in
// time is cut off, the connection closed. Once the connection's context has
// ended, Send fails with its cause and sends nothing.
func (c *Conn) Send(message []byte) error {
	if err := context.Cause(c.ctx); err != nil {
		return err
	}
	// The write is bound to the timeout alone: the library closes a
	// connection whose write is canceled, and a context ending in the middle
	// of a send, the server beginning to shut down, must leave the
	// connection whole for the status of its ending.
	ctx := context.WithoutCancel(c.ctx)
	if c.writeTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.writeTimeout)
		defer cancel()
	}
	if err := c.ws.Write(ctx, coderwebsocket.MessageText, message); err != nil {
		return errors.Wrap(err, "websocket: send")
	}
	return nil
}

// CloseSend tells the server the client has sent its last message, on a
// dialed connection: the server's Recv answers io.EOF from then on, and a
// message sent after it closes the connection.
func (c *Conn) CloseSend() error {
	if c.accepted {
		return errors.New("websocket: only a client ends its messages, a server closes the connection")
	}
	return c.Send([]byte(endOfMessages))
}

// CloseRecv tells the connection that Recv is called no more: Recv answers
// io.EOF from then on, and a message the peer sends from then on, or has sent
// and Recv has not taken, closes the connection with StatusPolicyViolation.
// An accepted connection still takes the frame its client ends its messages
// with. It is what the server of a stream whose client sends one message, or
// none, calls once it has what the client may send.
func (c *Conn) CloseRecv() {
	c.closeRecvOnce.Do(func() { close(c.recvClosed) })
}

// Close ends the connection with the status err stands for (see
// closeStatus), nil for an ending without an error, and waits for the peer to
// acknowledge it, briefly. Closing a connection again, or one that closed on
// what its peer sent, does nothing.
//
// It returns the status the connection closed with: the one err stands for,
// or the one the connection had closed with before, its peer's when the peer
// closed it, StatusAbnormalClosure, 1006, when the peer left without closing.
func (c *Conn) Close(err error) int {
	status, reason := closeStatus(err)
	c.shut(status, reason, errClosed)
	return int(c.status.Load())
}

// shut closes the connection with status and reason, the first time alone:
// its context ends with cause, the peer is told and given a moment to
// acknowledge, and an accepted connection is no longer one Drain waits for.
// The context ends before the peer is told, so that the cause is that of the
// close the peer reads, whatever notices the connection ending next.
func (c *Conn) shut(status coderwebsocket.StatusCode, reason string, cause error) {
	c.closeOnce.Do(func() {
		c.status.CompareAndSwap(0, int64(status))
		c.cancel(cause)
		// A peer already gone leaves nothing to tell.
		_ = c.ws.Close(status, reason)
		if c.stopWatch != nil {
			c.stopWatch()
		}
		if c.accepted {
			release(c)
		}
	})
}

// read reads the connection for as long as it lives and hands each message
// to Recv. It is the connection's only reader, which is what notices the
// peer leaving while no message waits for Recv, answers the peer's pings and
// reads the pongs its own are answered with.
func (c *Conn) read() {
	// finished: the client of an accepted connection has sent the frame that
	// ends its messages, and Recv has been told.
	finished := false
	end := func(cause error) {
		c.cancel(cause)
		if !finished {
			c.ended = context.Cause(c.ctx)
			close(c.messages)
		}
	}
	// violated closes the connection on what its peer may not send.
	violated := func(status coderwebsocket.StatusCode, reason string, cause error) {
		c.shut(status, reason, cause)
		end(cause)
	}
	// The reads are not bound to the end of the connection's context: the
	// library closes a connection whose read is canceled, before the status
	// of its ending could be sent.
	reading := context.WithoutCancel(c.ctx)
	for {
		kind, message, err := c.ws.Read(reading)
		switch {
		case err != nil && c.accepted && errors.Is(err, coderwebsocket.ErrMessageTooBig):
			// The library has closed the connection on the message itself.
			c.status.CompareAndSwap(0, int64(coderwebsocket.StatusMessageTooBig))
			end(errors.Wrap(err, "websocket: the client sent a message over the largest size"))
			return
		case err != nil:
			status := coderwebsocket.CloseStatus(err)
			if status == -1 {
				// The peer left without closing.
				status = coderwebsocket.StatusAbnormalClosure
			}
			c.status.CompareAndSwap(0, int64(status))
			if c.accepted {
				// The client left: the context ends canceled, the way a
				// request's does.
				end(nil)
			} else {
				end(closeError(err))
			}
			return
		case kind != coderwebsocket.MessageText:
			violated(coderwebsocket.StatusUnsupportedData, binaryReason, errBinaryMessage)
			return
		case c.accepted && string(bytes.TrimSpace(message)) == endOfMessages:
			if !finished {
				finished = true
				c.ended = io.EOF
				close(c.messages)
			}
		case finished:
			violated(coderwebsocket.StatusPolicyViolation, afterLastReason, errMessageAfterLast)
			return
		default:
			select {
			case c.messages <- message:
			case <-c.recvClosed:
				violated(coderwebsocket.StatusPolicyViolation, afterLastReason, errMessageAfterLast)
				return
			case <-c.ctx.Done():
				end(nil)
				return
			}
		}
	}
}

// beat pings the peer every interval until the connection's context ends, so
// that a connection carrying no message is not one a proxy on its way takes
// for idle. A ping that cannot be sent ends the context the way the peer
// leaving does: it is what notices a peer gone while a message it sent waits
// for Recv, when read reads nothing. A ping left unanswered is no verdict on
// the peer: its pong is read by read, which waits on Recv while a message is
// undelivered.
func (c *Conn) beat(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), interval)
			err := c.ws.Ping(ctx)
			unanswered := ctx.Err() != nil
			cancel()
			if err != nil && !unanswered {
				c.cancel(nil)
				return
			}
		}
	}
}

// closeStatus returns the status and the reason a connection closes with for
// the ending err of its stream: StatusNormalClosure for none;
// StatusGoingAway, with streamctx.ShutdownMsg, for the server shutting down;
// errorStatusBase plus its status, with its message, for a service error, 4404
// and "sample not found" for types.NewError(404, "sample not found"); and
// errorStatusBase plus 500 with types.FailureMsg for any other error, whose
// text stays out of the answer as it stays out of the HTTP one. The reason
// is the sentence in the language the process answers in, the one the HTTP
// envelope carries it in (see closeReason).
func closeStatus(err error) (coderwebsocket.StatusCode, string) {
	var serviceErr *types.Error
	switch {
	case err == nil:
		return coderwebsocket.StatusNormalClosure, ""
	case errors.Is(err, streamctx.ErrServerShutdown):
		return coderwebsocket.StatusGoingAway, closeReason(streamctx.ShutdownMsg)
	case errors.As(err, &serviceErr):
		return errorStatusBase + coderwebsocket.StatusCode(serviceErr.Status()), closeReason(serviceErr.Msg())
	default:
		return errorStatusBase + http.StatusInternalServerError, closeReason(types.FailureMsg)
	}
}

// closeReason returns the reason a close frame carries for msg: msg in the
// language the process answers in (see i18n.Translate), cut to what the
// frame holds, maxCloseReasonBytes, at the end of a character.
func closeReason(msg string) string {
	msg = i18n.Translate(msg)
	if len(msg) <= maxCloseReasonBytes {
		return msg
	}
	end := maxCloseReasonBytes
	for end > 0 && !utf8.RuneStart(msg[end]) {
		end--
	}
	return msg[:end]
}

// closeError returns what the Recv of a dialed connection answers once the
// connection ended with err, reading the ending of the stream back out of the
// status the server closed with, the reverse of closeStatus: io.EOF for
// StatusNormalClosure, the service being unavailable for StatusGoingAway, and
// the service error a status between 4400 and 4599 stands for, with the
// reason as its message. Any other ending is err itself, but for a
// connection that broke off without a close: the library reports it as an
// io.EOF, which Recv answers for a stream that is through, so it is
// io.ErrUnexpectedEOF here.
func closeError(err error) error {
	var closed coderwebsocket.CloseError
	if !errors.As(err, &closed) {
		if errors.Is(err, io.EOF) {
			return errors.Wrapf(io.ErrUnexpectedEOF, "websocket: the connection broke off: %v", err)
		}
		return errors.Wrap(err, "websocket: the connection ended")
	}
	switch {
	case closed.Code == coderwebsocket.StatusNormalClosure:
		return io.EOF
	case closed.Code == coderwebsocket.StatusGoingAway:
		return types.NewError(http.StatusServiceUnavailable, closed.Reason)
	case closed.Code >= errorStatusBase+http.StatusBadRequest && closed.Code <= errorStatusBase+599:
		return types.NewError(int(closed.Code-errorStatusBase), closed.Reason)
	default:
		return errors.Wrap(err, "websocket: the server closed the connection")
	}
}
