package document

import (
	"demo/model/archive"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*archive.Document, *archive.Document, *archive.Document]
}

func (d *Lister) List(ctx *gst.ServiceContext, req *archive.Document) (rsp *archive.Document, err error) {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("document list")
	return rsp, nil
}

func (d *Lister) ListBefore(ctx *gst.ServiceContext, documents *[]*archive.Document) error {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("document list before")
	return nil
}

func (d *Lister) ListAfter(ctx *gst.ServiceContext, documents *[]*archive.Document) error {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("document list after")
	return nil
}
