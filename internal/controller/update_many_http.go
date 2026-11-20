package controller

import (
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
)

// UpdateManyHandler returns a Gin handler that replaces multiple resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// batch[M], runs the batch update flow (see updateManyFlow), and
// returns the request data.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its UpdateMany method runs on the bound payload.
func UpdateManyHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.UpdateMany, apiaction.UpdateManyBefore, apiaction.UpdateManyAfter)
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

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.UpdateMany)

		var req batch[M]
		if reqErr := bindJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			response.Error(c, reportFailure(log, span, "bind request body failed", reqErr, invalidArgument(reqErr)))
			return
		}

		if err := a.updateManyFlow(requestContext(c), ginServiceContext(c), &req); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, req)
	}
}
