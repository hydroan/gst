package ticket

import (
	"net/http"

	"demo/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
)

// ownTicket reads the stored ticket id names and answers it only when it is
// the caller's: a ticket of someone else's, like one that does not exist,
// is not there, 404. The hook of the read and the hooks of every write share
// it, so a ticket is the caller's by one rule.
func ownTicket(ctx *gst.ServiceContext, id string) (*model.Ticket, error) {
	stored := new(model.Ticket)
	if err := database.Database[*model.Ticket](ctx).Get(stored, id); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, gst.NewErrorWithCause(http.StatusNotFound, "ticket not found", err)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to load the ticket", err)
	}
	if stored.UserID != ctx.UserID() {
		return nil, gst.NewError(http.StatusNotFound, "ticket not found")
	}
	return stored, nil
}
