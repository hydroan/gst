package document

import (
	"demo/model/archive"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Updater struct {
	service.Base[*archive.Document, *archive.Document, *archive.Document]
}

func (d *Updater) Update(ctx *gst.ServiceContext, req *archive.Document) (rsp *archive.Document, err error) {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("document update")
	return rsp, nil
}

func (d *Updater) UpdateBefore(ctx *gst.ServiceContext, document *archive.Document) error {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("document update before")
	return nil
}

func (d *Updater) UpdateAfter(ctx *gst.ServiceContext, document *archive.Document) error {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("document update after")
	return nil
}
