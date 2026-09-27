package item

import (
	"demo/model/record"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Lister hooks the framework's own List of items.
type Lister struct {
	service.Base[*record.Item, *record.Item, *record.Item]
}

// Filter keeps the list to the items of the parent the route names.
func (i *Lister) Filter(ctx *gst.ServiceContext, item *record.Item, opts gst.QueryOptions) (*record.Item, gst.QueryOptions, error) {
	item.RecordID = ctx.Param("record")
	return item, opts, nil
}
