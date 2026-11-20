package ticket

import (
	"net/http"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
)

// Creator serves POST /api/tickets: an action with request and response
// types of its own, so the body carries the fields a client may set, the
// title and the status, and the service builds the row around them.
type Creator struct {
	service.Base[*model.Ticket, *model.TicketCreateReq, *model.TicketCreateRsp]
}

// Create files the ticket for the caller: the owner is the session's user,
// which the request has no field to name, the status defaults to open, and
// the creation leaves an audit row. Both writes go through the database
// chain, on the request's context. An error a service returns is a
// gst.Error: it names the status the client gets and the message, and
// carries the cause for the log.
func (r *Creator) Create(ctx *gst.ServiceContext, req *model.TicketCreateReq) (*model.TicketCreateRsp, error) {
	ticket := &model.Ticket{UserID: ctx.UserID(), Title: req.Title, Status: req.Status}
	if ticket.Status == "" {
		ticket.Status = model.TicketStatusOpen
	}
	if err := database.Database[*model.Ticket](ctx).Create(ticket); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to create the ticket", err)
	}
	audit := &model.Audit{Action: "create", TicketID: ticket.ID, Actor: ctx.Username()}
	if err := database.Database[*model.Audit](ctx).Create(audit); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to write the audit row", err)
	}
	return ticket, nil
}
