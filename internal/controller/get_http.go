package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
)

// GetHandler returns a Gin handler that retrieves one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the configured
// route parameter as the resource id, runs the get flow (see getFlow), and
// returns the model.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Get method runs on a zero-value REQ, the GET request
// carrying no body; the service reads ServiceContext.Query() and Param().
func GetHandler[M gst.Model, REQ gst.Request, RSP gst.Response](cfg ...*gst.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.Get, apiaction.GetBefore, apiaction.GetAfter)
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

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.Get)
		id, ok := routeID(c, cfg, log, span)
		if !ok {
			return
		}

		m, err := a.getFlow(requestContext(c), ginServiceContext(c), id)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, m)
	}
}
