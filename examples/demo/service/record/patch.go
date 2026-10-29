package record

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Patcher hooks the framework's own Patch, which writes the stored row with
// the fields the body names merged in: the hook keeps the patch to the
// caller's own records and the owner what it was, whatever the body named.
type Patcher struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

// PatchBefore keeps the record the caller's (see ownRecord) and restores
// the owner into the merged row, a body naming user_id having changed it.
func (r *Patcher) PatchBefore(ctx *gst.ServiceContext, record *model.Record) error {
	stored, err := ownRecord(ctx, record.ID)
	if err != nil {
		return err
	}
	record.UserID = stored.UserID
	return nil
}
