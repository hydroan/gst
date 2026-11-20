package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
)

// UpdateHandler returns a Gin handler that replaces one resource.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// M, reads the resource id from the configured route parameter (the id carried
// by the body is ignored), and runs the update flow (see updateFlow), which
// answers with the replacement.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Update method runs on the bound payload.
func UpdateHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.Update, apiaction.UpdateBefore, apiaction.UpdateAfter)
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

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.Update)

		req := a.newModel()
		if reqErr := bindJSONRequest(c, &req); reqErr != nil {
			// A full update replaces the resource, so an absent body is refused
			// rather than tolerated as "nothing to change".
			reqErr = requiredBodyError(reqErr)
			response.Error(c, reportFailure(log, span, "bind request body failed", reqErr, invalidArgument(reqErr)))
			return
		}

		id, ok := routeID(c, cfg, log, span)
		if !ok {
			return
		}

		if err := a.updateFlow(requestContext(c), ginServiceContext(c), id, req); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, req)
	}
}
