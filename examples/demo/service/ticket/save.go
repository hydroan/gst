package ticket

import (
	"net/http"

	"demo/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
)

// saveTicket writes the ticket back under the version it carries, the one
// the client read: the write refuses a version the row has moved past,
// answered 409, and a ticket carrying none, answered 400, so two edits of
// one ticket never overwrite each other unseen. The update and the patch
// share it, so a version is checked by one rule.
func saveTicket(ctx *gst.ServiceContext, ticket *model.Ticket) error {
	err := database.Database[*model.Ticket](ctx).Update(ticket)
	switch {
	case errors.Is(err, database.ErrStaleObject):
		return gst.NewErrorWithCause(http.StatusConflict, "the ticket was changed since it was read, reload it and retry", err)
	case errors.Is(err, database.ErrVersionRequired):
		return gst.NewErrorWithCause(http.StatusBadRequest, "the version the ticket was read at is required", err)
	case err != nil:
		return gst.NewErrorWithCause(http.StatusInternalServerError, "failed to save the ticket", err)
	}
	return nil
}
