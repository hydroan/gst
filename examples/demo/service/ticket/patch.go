package ticket

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Patcher hooks the framework's own Patch, which writes the stored row with
// the fields the body names merged in: the hook keeps the patch to the
// caller's own tickets and the owner what it was, whatever the body named.
type Patcher struct {
	service.Base[*model.Ticket, *model.Ticket, *model.Ticket]
}

// PatchBefore keeps the ticket the caller's (see ownTicket) and restores
// the owner into the merged row, a body naming user_id having changed it.
func (r *Patcher) PatchBefore(ctx *gst.ServiceContext, ticket *model.Ticket) error {
	stored, err := ownTicket(ctx, ticket.ID)
	if err != nil {
		return err
	}
	ticket.UserID = stored.UserID
	return nil
}
