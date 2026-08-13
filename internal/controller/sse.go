package controller

import (
	"context"

	"github.com/gin-gonic/gin"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// SSE handles a Server-Sent Events request with the default factory settings.
func SSE[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	SSEFactory[M, REQ, RSP]()(c)
}

// SSEFactory returns a Gin handler that streams Server-Sent Events.
//
// The action always delegates to the phase service's SSE method, which opens
// the stream through ServiceContext.SSE and blocks until it is over; there is
// no default streaming behavior. The handler distinguishes the two failure
// shapes by whether the response was written: a setup failure before the
// stream opened is answered as a regular error envelope, while an error after
// streaming began can only be logged, because the response is already on the
// wire.
func SSEFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_SSE)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_SSE)
		if err := meta.traceServiceHook(ctrlSpanCtx, consts.PHASE_SSE, func(spanCtx context.Context) error {
			return meta.service().SSE(types.NewServiceContext(c, spanCtx, consts.PHASE_SSE))
		}); err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			gstotel.RecordError(span, err)
			if !c.Writer.Written() {
				handleServiceError(c, err)
			}
			return
		}
		// A finished stream needs no envelope; the connection closing is the
		// response. A service that never opened the stream and returned nil
		// still owes the client an answer.
		if !c.Writer.Written() {
			JSON(c, CodeSuccess, nil)
		}
	}
}
