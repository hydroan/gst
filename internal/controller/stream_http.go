package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/streamctx"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/websocket"
	"github.com/hydroan/gst/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// This file holds the handler of a Stream action over HTTP, the counterpart
// of the stream calls gRPC serves the action through (see stream_grpc.go;
// stream.go holds what the two share). The stream travels on a WebSocket
// connection, every message one text frame holding a JSON document (see the
// websocket package): the request of a server stream is the first message
// the client sends, the requests of the other two kinds are the client's
// messages up to the frame that ends them, and every response is a message
// the server sends. A request is decoded, normalized and validated the way a
// request body is bound (see bindJSONRequest), each one of a request stream
// as it is read.

// errServicePanicked closes, as the server's own failure, the connection of a
// stream whose service panicked (see StreamHandler).
var errServicePanicked = errors.New("controller: the service of the stream panicked")

// StreamHandler returns a Gin handler that serves a Stream action over a
// WebSocket connection.
//
// The request is the handshake, and until the connection is taken over it is
// answered like any request, with a regular error envelope: a route parameter
// the action may not carry is refused with 400, a service declaring no Stream
// method of the action's kind (see types.ServerStreamer and its kind) with
// 501 and unimplementedMsg, and a handshake the protocol or the origins
// admitted (config.Server.WebSocketOrigins) refuse with the status and the
// text of the refusal (see websocket.Accept).
//
// Once the connection is open the stream's ending is the close of the
// connection (see websocket.Conn.Close). A service returning without an error
// closes it normally, the response of a client stream sent first. A service
// error closes it with the status standing for the HTTP status the error
// answers with, carrying its message, and any other error as the server's own
// failure. A message the decoding or the validator refuses closes it as the
// 400 the refusal is, whatever the service returns for the error its Recv
// gave it, and the request of a server stream that does not arrive within
// server.read_timeout as a 408.
//
// The whole stream runs on the connection's context (see
// websocket.Conn.Context): the client leaving or the server beginning to shut
// down ends the service's work with the stream. A stream ending that way is
// how streams end, not a failure: the span records it as an interrupted
// event with the reason, a service returning nothing but that ending is
// neither logged nor recorded as an error, and a stream the shutdown ended
// tells its client the server is going away. A service that panics leaves
// the connection closed as the server's own failure, before the recovery up
// the chain takes the panic. The status the connection closed with is left
// on the gin context under requestctx.KeyCloseCode, for the access log.
func StreamHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	route := routeFromConfig(cfg...)
	a := newAction[M, REQ, RSP](route, apiaction.Stream)
	noRequest, noResponse := model.IsEmpty[REQ](), model.IsEmpty[RSP]()
	return func(c *gin.Context) {
		_, span, ok := a.beginRequest(c)
		defer span.End()
		if !ok {
			return
		}
		log := logger.Controller.WithContext(c.Request.Context(), apiaction.Stream)

		svc := a.service()
		server, isServer := svc.(types.ServerStreamer[REQ, RSP])
		client, isClient := svc.(types.ClientStreamer[REQ, RSP])
		bidi, isBidi := svc.(types.BidiStreamer[REQ, RSP])
		if !isServer && !isClient && !isBidi {
			err := a.unserved(route)
			response.Error(c, reportFailure(log, span, "service operation failed", err, types.NewErrorWithCause(http.StatusNotImplemented, unimplementedMsg, err)))
			return
		}

		conn, err := websocket.Accept(c.Writer, c.Request, config.App.Server.WebSocketOrigins, config.App.Server.WriteTimeout)
		if err != nil {
			response.Error(c, reportFailure(log, span, "request rejected", err, err))
			return
		}
		// Nothing but the handler closes a connection taken over from the
		// server, so it is closed however the handler ends: the close
		// deferred here is the one a service that panics leaves it to, and
		// only reads the status once the ending below has closed it.
		defer func() { c.Set(requestctx.KeyCloseCode, conn.Close(errServicePanicked)) }()
		stream := conn.Context()
		c.Request = c.Request.WithContext(stream)

		send := func(rsp RSP) error {
			message, encodeErr := json.Marshal(rsp)
			if encodeErr != nil {
				return errors.Wrap(encodeErr, "encode the response")
			}
			return conn.Send(message)
		}
		// in reads the requests of a request stream, keeping the refusal of
		// one the decoding or the validator refuses for the ending below.
		in := &requests[REQ]{}
		in.recv = func() (REQ, error) {
			raw, recvErr := conn.Recv()
			if recvErr != nil {
				var zero REQ
				return zero, recvErr
			}
			req, refusal := a.decodeMessage(raw)
			if refusal != nil {
				in.refuse(refusal)
			}
			return req, refusal
		}

		var req REQ
		if isServer {
			// A request the stream's context ended the wait for is no refusal:
			// the stream ended before it began.
			if req, err = a.awaitRequest(conn, noRequest, config.App.Server.ReadTimeout); err != nil && stream.Err() == nil {
				in.refuse(err)
			}
		}
		var rsp RSP
		if err == nil {
			rsp, err = a.traceServiceOperation(stream, apiaction.Stream, func(spanCtx context.Context) (RSP, error) {
				var zero RSP
				sc := types.NewServiceContext(c, spanCtx, apiaction.Stream)
				switch {
				case isServer:
					return zero, server.Stream(sc, req, types.NewServerStream(send))
				case isClient:
					return client.Stream(sc, types.NewClientStream(in.recv))
				default:
					return zero, bidi.Stream(sc, types.NewBidiStream(in.recv, send))
				}
			})
		}

		// ending is what the connection closes with, the endings in the order
		// one takes precedence over the next.
		var ending error
		switch refused := in.refusal(); {
		case refused != nil:
			ending = reportFailure(log, span, "request rejected", refused, refused)
		case stream.Err() != nil:
			cause := context.Cause(stream)
			shutdown := errors.Is(cause, streamctx.ErrServerShutdown)
			if !shutdown && !errors.Is(cause, stream.Err()) {
				// What the client sent closed the connection, with the status
				// the protocol gives it (see websocket.Conn): the client's
				// doing, reported as the refusal of a request.
				_ = reportFailure(log, span, "request rejected", cause, invalidArgument(cause))
				break
			}
			span.AddEvent("interrupted", trace.WithAttributes(attribute.String("reason", streamEndReason(stream))))
			if err != nil && !lifecycle.Interrupted(stream, err) {
				// A failure of the service's own beside the ending is still
				// reported, though the ending is what the client is told.
				_ = failService(stream, log, err)
			}
			if shutdown {
				ending = cause
			}
		case err != nil:
			ending = failService(stream, log, err)
		case isClient && !noResponse:
			if err = send(rsp); err != nil {
				ending = failService(stream, log, err)
			}
		}
		conn.Close(ending)
	}
}

// decodeMessage returns the request the message raw of a stream holds,
// decoded, normalized and validated the way a request body is bound (see
// bindJSONRequest): a message the decoding or the validator refuses is
// refused with the client-safe 400 a body it refuses is answered with. A nil
// raw is no message, and returns the zero request, normalized and validated
// all the same.
func (a *action[M, REQ, RSP]) decodeMessage(raw []byte) (REQ, error) {
	req := a.newRequest()
	if raw != nil {
		if err := decodeJSON(raw, &req); err != nil {
			return req, err
		}
	}
	a.normalizeRequest(&req)
	if err := validateRequest(req); err != nil {
		return req, clientSafeBindError(err)
	}
	return req, nil
}

// awaitRequest returns the request of a server stream on conn, and ends what
// its client may send (see websocket.Conn.CloseRecv). The request is the
// first message of the connection, awaited no longer than timeout, zero for
// no bound: a client sending none in time is refused with 408 and
// requestTimeoutMsg. It is the zero request when the client ends its
// messages without sending one, and when the action declares no request,
// noRequest, in which case none is awaited.
func (a *action[M, REQ, RSP]) awaitRequest(conn *websocket.Conn, noRequest bool, timeout time.Duration) (REQ, error) {
	var raw []byte
	if !noRequest {
		type received struct {
			message []byte
			err     error
		}
		first := make(chan received, 1)
		go func() {
			message, err := conn.Recv()
			first <- received{message: message, err: err}
		}()
		var expired <-chan time.Time
		if timeout > 0 {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			expired = timer.C
		}
		select {
		case got := <-first:
			if got.err != nil && !errors.Is(got.err, io.EOF) {
				var zero REQ
				return zero, got.err
			}
			raw = got.message
		case <-expired:
			var zero REQ
			return zero, types.NewError(http.StatusRequestTimeout, requestTimeoutMsg)
		}
	}
	conn.CloseRecv()
	return a.decodeMessage(raw)
}
