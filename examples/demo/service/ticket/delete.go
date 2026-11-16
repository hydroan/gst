package ticket

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Deleter hooks the framework's own Delete: the hook keeps the delete to
// the caller's own tickets.
type Deleter struct {
	service.Base[*model.Ticket, *model.Ticket, *model.Ticket]
}

// DeleteBefore keeps the ticket the caller's (see ownTicket); the body of a
// delete names the id alone.
func (r *Deleter) DeleteBefore(ctx *gst.ServiceContext, ticket *model.Ticket) error {
	_, err := ownTicket(ctx, ticket.ID)
	return err
}
