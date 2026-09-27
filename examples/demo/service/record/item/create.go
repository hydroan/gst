package item

import (
	"demo/model/record"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Creator hooks the framework's own Create of an item.
type Creator struct {
	service.Base[*record.Item, *record.Item, *record.Item]
}

// CreateBefore takes the parent from the route, /api/records/:record/items:
// the route's parameters are read with ctx.Param.
func (i *Creator) CreateBefore(ctx *gst.ServiceContext, item *record.Item) error {
	item.RecordID = ctx.Param("record")
	return nil
}
