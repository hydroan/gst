package record

import (
	"net/http"

	"demo/model"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
)

// ownRecord reads the stored record id names and answers it only when it is
// the caller's: a record of someone else's, like one that does not exist,
// is not there, 404. The hooks of every write share it, so a record is the
// caller's by one rule.
func ownRecord(ctx *gst.ServiceContext, id string) (*model.Record, error) {
	stored := new(model.Record)
	if err := database.Database[*model.Record](ctx).Get(stored, id); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, gst.NewErrorWithCause(http.StatusNotFound, "record not found", err)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to load the record", err)
	}
	if stored.UserID != ctx.UserID() {
		return nil, gst.NewError(http.StatusNotFound, "record not found")
	}
	return stored, nil
}
