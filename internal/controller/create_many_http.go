package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
	"github.com/hydroan/gst/internal/response"
)

// CreateManyHandler returns a Gin handler that creates multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// batch[M], runs the batch create flow (see createManyFlow), and
// returns the request data. A batch create requires a body: an absent one is
// refused, like a body whose items the flow finds empty.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its CreateMany method runs on the bound payload.
func CreateManyHandler[M gst.Model, REQ gst.Request, RSP gst.Response](cfg ...*gst.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.CreateMany, apiaction.CreateManyBefore, apiaction.CreateManyAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span, ok := a.beginRequest(c)
		defer span.End()
		if !ok {
			return
		}

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.CreateMany)

		var req batch[M]
		if reqErr := bindJSONRequest(c, &req); reqErr != nil {
			// A batch create carries the items to create, so an absent body is
			// refused rather than answered as a success that wrote nothing.
			reqErr = requiredBodyError(reqErr)
			response.Error(c, reportFailure(log, span, "bind request body failed", reqErr, invalidArgument(reqErr)))
			return
		}

		if err := a.createManyFlow(requestContext(c), ginServiceContext(c), &req); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, req)
	}
}
