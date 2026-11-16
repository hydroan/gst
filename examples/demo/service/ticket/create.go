package ticket

import (
	"net/http"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
)

// Creator hooks the framework's own Create: the model, request and response
// types are the same, so the framework reads the body, writes the row and
// answers it, and calls the hooks around that.
type Creator struct {
	service.Base[*model.Ticket, *model.Ticket, *model.Ticket]
}

// CreateBefore stamps the owner and the default status: a ticket belongs to
// whoever creates it, whatever the body says, and the session names them.
func (r *Creator) CreateBefore(ctx *gst.ServiceContext, ticket *model.Ticket) error {
	ticket.UserID = ctx.UserID()
	if ticket.Status == "" {
		ticket.Status = model.TicketStatusOpen
	}
	return nil
}

// CreateAfter writes the audit row of the creation through the database
// chain, on the request's context. An error a service returns is a
// gst.Error: it names the status the client gets and the message, and
// carries the cause for the log.
func (r *Creator) CreateAfter(ctx *gst.ServiceContext, ticket *model.Ticket) error {
	audit := &model.Audit{Action: "create", TicketID: ticket.ID, Actor: ctx.Username()}
	if err := database.Database[*model.Audit](ctx).Create(audit); err != nil {
		return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to write the audit row", err)
	}
	return nil
}
