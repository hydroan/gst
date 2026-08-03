package record

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Patcher struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

func (c *Patcher) Patch(ctx *types.ServiceContext, req *model.Record) (rsp *model.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record patch")
	return rsp, nil
}

func (c *Patcher) PatchBefore(ctx *types.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record patch before")
	return nil
}

func (c *Patcher) PatchAfter(ctx *types.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record patch after")
	return nil
}
