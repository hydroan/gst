package document

import (
	"demo/model/archive"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*archive.Document, *archive.Document, *archive.Document]
}

func (d *Creator) Create(ctx *gst.ServiceContext, req *archive.Document) (rsp *archive.Document, err error) {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("document create")
	return rsp, nil
}

func (d *Creator) CreateBefore(ctx *gst.ServiceContext, document *archive.Document) error {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("document create before")
	return nil
}

func (d *Creator) CreateAfter(ctx *gst.ServiceContext, document *archive.Document) error {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("document create after")
	return nil
}
