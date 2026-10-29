package record

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Updater hooks the framework's own Update, which replaces the row with the
// body: the hook keeps the update to the caller's own records and the owner
// what it was. The version check is the framework's, at the write that
// follows the hook: a body carrying a stale version answers 409, one
// carrying none 400.
type Updater struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

// UpdateBefore keeps the record the caller's (see ownRecord) and restores
// the owner into the body, which names none or another.
func (r *Updater) UpdateBefore(ctx *gst.ServiceContext, record *model.Record) error {
	stored, err := ownRecord(ctx, record.ID)
	if err != nil {
		return err
	}
	record.UserID = stored.UserID
	return nil
}
