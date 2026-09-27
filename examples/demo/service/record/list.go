package record

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Lister hooks the framework's own List.
type Lister struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

// Filter keeps a list to the caller's own records: record carries the
// conditions the query parameters set, and the owner is added to them
// whatever the client asked for.
func (r *Lister) Filter(ctx *gst.ServiceContext, record *model.Record, opts gst.QueryOptions) (*model.Record, gst.QueryOptions, error) {
	record.UserID = ctx.UserID()
	return record, opts, nil
}

// ListAfter fills the owner's name in for the client, the rows being the
// caller's own.
func (r *Lister) ListAfter(ctx *gst.ServiceContext, records *[]*model.Record) error {
	for _, record := range *records {
		record.Username = ctx.Username()
	}
	return nil
}
