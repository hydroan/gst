package controller

import (
	"context"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/sse"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SSEHandler returns a Gin handler that streams Server-Sent Events.
//
// The action always delegates to the phase service's SSE method, which opens
// the stream through ServiceContext.SSE and blocks until it is over; there is
// no default streaming behavior. The whole request runs on the stream's
// context, see sse.StreamContext: the service's own work and the stream end
// together when the client leaves or the server begins to shut down, so an
// open stream never holds the shutdown.
//
// A stream ending that way is how streams end, not a failure. The span
// records how it ended as an interrupted event with the reason, and a service
// returning nothing but that ending is neither logged nor recorded as an
// error, on this span or on the service's; a stream that ended that way
// before it opened is answered by what ended it, the server shutting down
// with 503, a client that went away as a canceled request. A failure of its
// own is reported the way every flow reports one (see failService): the
// handler distinguishes the two failure shapes by whether the response was
// written. A setup failure before the stream opened is answered as a regular
// error envelope, while an error after streaming began can only be logged,
// because the response is already on the wire.
func SSEHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.SSE)
	return func(c *gin.Context) {
		stream, stopStream := sse.StreamContext(c.Request.Context())
		defer stopStream()
		c.Request = c.Request.WithContext(stream)

		ctrlSpanCtx, span, ok := a.beginRequest(c)
		defer span.End()
		if !ok {
			return
		}

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.SSE)
		svc := a.service()
		// ended keeps what the service returned when that was nothing but the
		// stream's context ending, away from the service span and the log.
		var ended error
		err := a.traceServiceHook(ctrlSpanCtx, apiaction.SSE, svc, ginServiceContext(c), func(sc *types.ServiceContext) error {
			err := svc.SSE(sc)
			if lifecycle.Interrupted(stream, err) {
				ended = err
				return nil
			}
			return err
		})
		if reason := streamEndReason(stream); reason != "" {
			span.AddEvent("interrupted", trace.WithAttributes(attribute.String("reason", reason)))
		}
		switch {
		case err != nil:
			err = failService(ctrlSpanCtx, log, err)
			if !c.Writer.Written() {
				response.Error(c, err)
			}
		case c.Writer.Written():
			// A finished stream needs no envelope; the connection closing is
			// the response.
		case ended != nil:
			// A stream that ended before it opened is answered by what ended
			// it: the server shutting down as unavailable, the status gRPC
			// answers a stream the stop ended, so that the client resumes on
			// another replica, and the client going away as a canceled
			// request.
			answer := endedAnswer(stream.Err(), ended)
			if errors.Is(context.Cause(stream), sse.ErrServerShutdown) {
				answer = types.NewErrorWithCause(http.StatusServiceUnavailable, shutdownMsg, ended)
			}
			response.Error(c, answer)
		default:
			// A service that never opened the stream and returned nil still
			// owes the client an answer.
			response.JSON(c)
		}
	}
}

// streamEndReason names why the stream's context ended — the server began to
// shut down, or the client went away — and is empty while it has not.
func streamEndReason(stream context.Context) string {
	switch {
	case stream.Err() == nil:
		return ""
	case errors.Is(context.Cause(stream), sse.ErrServerShutdown):
		return "server shutting down"
	default:
		return "client disconnected"
	}
}
