package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
	"github.com/hydroan/gst/internal/response"
)

// DeleteHandler returns a Gin handler that deletes one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the resource id
// from the configured route parameter (batch deletion uses the DeleteMany
// action instead), runs the delete flow (see deleteFlow), and returns a
// success response.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Delete method runs on the bound payload.
func DeleteHandler[M gst.Model, REQ gst.Request, RSP gst.Response](cfg ...*gst.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.Delete, apiaction.DeleteBefore, apiaction.DeleteAfter)
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

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.Delete)
		id, ok := routeID(c, cfg, log, span)
		if !ok {
			return
		}

		if err := a.deleteFlow(requestContext(c), ginServiceContext(c), id); err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c)
	}
}
