package record

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Deleter hooks the framework's own Delete: the hook keeps the delete to
// the caller's own records.
type Deleter struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

// DeleteBefore keeps the record the caller's (see ownRecord); the body of a
// delete names the id alone.
func (r *Deleter) DeleteBefore(ctx *gst.ServiceContext, record *model.Record) error {
	_, err := ownRecord(ctx, record.ID)
	return err
}
