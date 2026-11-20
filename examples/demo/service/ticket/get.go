package ticket

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Getter hooks the framework's own Get: the hook keeps the read to the
// caller's own tickets.
type Getter struct {
	service.Base[*model.Ticket, *model.Ticket, *model.Ticket]
}

// GetBefore keeps the ticket the caller's (see ownTicket), before the
// framework reads it: the model of a read names the id alone, and checking
// here answers someone else's ticket exactly as a missing one, the same
// status and the same message.
func (r *Getter) GetBefore(ctx *gst.ServiceContext, ticket *model.Ticket) error {
	_, err := ownTicket(ctx, ticket.ID)
	return err
}
