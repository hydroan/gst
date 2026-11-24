package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
)

// CreateHandler returns a Gin handler that creates one resource.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// M, runs the create flow (see createFlow), and returns the created model.
// Creating a resource requires a body: an absent one is refused, and a client
// wanting a resource with all defaults states that intent with an explicit {}
// body.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Create method runs on the bound payload, a multipart
// form left unbound for the service to read itself.
func CreateHandler[M gst.Model, REQ gst.Request, RSP gst.Response](cfg ...*gst.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.Create, apiaction.CreateBefore, apiaction.CreateAfter)
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

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.Create)

		req := a.newModel()
		if reqErr := bindJSONRequest(c, &req); reqErr != nil {
			// Creating a resource requires a body, so an absent one is refused
			// rather than answered as a success that wrote nothing.
			reqErr = requiredBodyError(reqErr)
			response.Error(c, reportFailure(log, span, "bind request body failed", reqErr, invalidArgument(reqErr)))
			return
		}

		if err := a.createFlow(requestContext(c), ginServiceContext(c), req); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, req)
	}
}
