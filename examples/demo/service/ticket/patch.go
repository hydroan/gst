package ticket

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Patcher serves PATCH /api/tickets/:ticket: an action with a request type
// of its own, the fields a client may change, each optional, and the
// version it read.
type Patcher struct {
	service.Base[*model.Ticket, *model.TicketPatchReq, *model.TicketPatchRsp]
}

// Patch changes the fields the body names of the caller's ticket (see
// ownTicket), which the route names, leaves the others as stored, and
// writes the ticket back under the version the body carries (see
// saveTicket).
func (r *Patcher) Patch(ctx *gst.ServiceContext, req *model.TicketPatchReq) (*model.TicketPatchRsp, error) {
	ticket, err := ownTicket(ctx, ctx.Param("ticket"))
	if err != nil {
		return nil, err
	}
	if req.Title != nil {
		ticket.Title = *req.Title
	}
	if req.Status != nil {
		ticket.Status = *req.Status
	}
	ticket.Version = req.Version
	if err := saveTicket(ctx, ticket); err != nil {
		return nil, err
	}
	return ticket, nil
}
