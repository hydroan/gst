package record

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

func (c *Lister) List(ctx *gst.ServiceContext, req *model.Record) (rsp *model.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record list")
	return rsp, nil
}

func (c *Lister) ListBefore(ctx *gst.ServiceContext, records *[]*model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record list before")
	return nil
}

func (c *Lister) ListAfter(ctx *gst.ServiceContext, records *[]*model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record list after")
	return nil
}
