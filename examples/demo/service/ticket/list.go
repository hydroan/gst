package ticket

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Lister hooks the framework's own List.
type Lister struct {
	service.Base[*model.Ticket, *model.Ticket, *model.Ticket]
}

// Filter keeps a list to the caller's own tickets: ticket carries the
// conditions the query parameters set, and the owner is added to them
// whatever the client asked for.
func (r *Lister) Filter(ctx *gst.ServiceContext, ticket *model.Ticket, opts gst.QueryOptions) (*model.Ticket, gst.QueryOptions, error) {
	ticket.UserID = ctx.UserID()
	return ticket, opts, nil
}

// ListAfter fills the owner's name in for the client, the rows being the
// caller's own.
func (r *Lister) ListAfter(ctx *gst.ServiceContext, tickets *[]*model.Ticket) error {
	for _, ticket := range *tickets {
		ticket.Username = ctx.Username()
	}
	return nil
}
