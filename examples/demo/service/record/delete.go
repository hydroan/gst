package record

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Deleter struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

func (c *Deleter) Delete(ctx *gst.ServiceContext, req *model.Record) (rsp *model.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record delete")
	return rsp, nil
}

func (c *Deleter) DeleteBefore(ctx *gst.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record delete before")
	return nil
}

func (c *Deleter) DeleteAfter(ctx *gst.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record delete after")
	return nil
}
