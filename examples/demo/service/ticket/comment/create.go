package comment

import (
	"demo/model/ticket"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Creator hooks the framework's own Create of a comment.
type Creator struct {
	service.Base[*ticket.Comment, *ticket.Comment, *ticket.Comment]
}

// CreateBefore takes the parent from the route, /api/tickets/:ticket/comments:
// the route's parameters are read with ctx.Param.
func (i *Creator) CreateBefore(ctx *gst.ServiceContext, comment *ticket.Comment) error {
	comment.TicketID = ctx.Param("ticket")
	return nil
}
