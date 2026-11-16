package comment

import (
	"demo/model/ticket"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Lister hooks the framework's own List of comments.
type Lister struct {
	service.Base[*ticket.Comment, *ticket.Comment, *ticket.Comment]
}

// Filter keeps the list to the comments of the parent the route names.
func (i *Lister) Filter(ctx *gst.ServiceContext, comment *ticket.Comment, opts gst.QueryOptions) (*ticket.Comment, gst.QueryOptions, error) {
	comment.TicketID = ctx.Param("ticket")
	return comment, opts, nil
}
