package record

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Deleter struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

func (c *Deleter) Delete(ctx *types.ServiceContext, req *model.Record) (rsp *model.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record delete")
	return rsp, nil
}

func (c *Deleter) DeleteBefore(ctx *types.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record delete before")
	return nil
}

func (c *Deleter) DeleteAfter(ctx *types.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record delete after")
	return nil
}
