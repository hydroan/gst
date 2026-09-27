package document

import (
	"demo/model/archive"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// ListByBox hooks the framework's own List on the box route,
// /api/archive/boxes/:box_id/documents: the route names the service, since
// the documents route has a List of its own.
type ListByBox struct {
	service.Base[*archive.Document, *archive.Document, *archive.Document]
}

// Filter keeps the list to the documents of the box the route names.
func (l *ListByBox) Filter(ctx *gst.ServiceContext, document *archive.Document, opts gst.QueryOptions) (*archive.Document, gst.QueryOptions, error) {
	document.BoxID = ctx.Param("box_id")
	return document, opts, nil
}
