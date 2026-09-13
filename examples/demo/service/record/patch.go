package record

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Patcher struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

func (c *Patcher) Patch(ctx *gst.ServiceContext, req *model.Record) (rsp *model.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record patch")
	return rsp, nil
}

func (c *Patcher) PatchBefore(ctx *gst.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record patch before")
	return nil
}

func (c *Patcher) PatchAfter(ctx *gst.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record patch after")
	return nil
}
