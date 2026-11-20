package ticket

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Updater serves PUT /api/tickets/:ticket: an action with a request type of
// its own, the fields a client may replace and the version it read.
type Updater struct {
	service.Base[*model.Ticket, *model.TicketUpdateReq, *model.TicketUpdateRsp]
}

// Update replaces the title and the status of the caller's ticket (see
// ownTicket), which the route names, and writes it back under the version
// the body carries (see saveTicket).
func (r *Updater) Update(ctx *gst.ServiceContext, req *model.TicketUpdateReq) (*model.TicketUpdateRsp, error) {
	ticket, err := ownTicket(ctx, ctx.Param("ticket"))
	if err != nil {
		return nil, err
	}
	ticket.Title, ticket.Status, ticket.Version = req.Title, req.Status, req.Version
	if err := saveTicket(ctx, ticket); err != nil {
		return nil, err
	}
	return ticket, nil
}
