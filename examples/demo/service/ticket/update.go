package ticket

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Updater hooks the framework's own Update, which replaces the row with the
// body: the hook keeps the update to the caller's own tickets and the owner
// what it was. The version check is the framework's, at the write that
// follows the hook: a body carrying a stale version answers 409, one
// carrying none 400.
type Updater struct {
	service.Base[*model.Ticket, *model.Ticket, *model.Ticket]
}

// UpdateBefore keeps the ticket the caller's (see ownTicket) and restores
// the owner into the body, which names none or another.
func (r *Updater) UpdateBefore(ctx *gst.ServiceContext, ticket *model.Ticket) error {
	stored, err := ownTicket(ctx, ticket.ID)
	if err != nil {
		return err
	}
	ticket.UserID = stored.UserID
	return nil
}
