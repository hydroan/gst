package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
)

// ListHandler returns a Gin handler that lists resources.
//
// When M, REQ, and RSP are the same type, the handler runs the list flow (see
// listFlow) and returns the items with a total count, zero under cursor
// pagination, which counts nothing, and with has_more, whether records lie
// past the page: before it when the cursor is read backward, after it
// otherwise.
//
// The automatic listing branch supports model schema fields plus framework
// query parameters for pagination, cursor pagination, expansion, depth,
// ordering, and field operator filters.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its List method runs on a zero-value REQ, the GET request
// carrying no body; the service reads ServiceContext.Query().
func ListHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.List, apiaction.ListBefore, apiaction.ListAfter)
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

		items, total, hasMore, err := a.listFlow(requestContext(c), ginServiceContext(c))
		if err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, gin.H{
			"items":    items,
			"total":    total,
			"has_more": hasMore,
		})
	}
}
