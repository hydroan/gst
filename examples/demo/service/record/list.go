package record

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

func (c *Lister) List(ctx *types.ServiceContext, req *model.Record) (rsp *model.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record list")
	return rsp, nil
}

func (c *Lister) ListBefore(ctx *types.ServiceContext, records *[]*model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record list before")
	return nil
}

func (c *Lister) ListAfter(ctx *types.ServiceContext, records *[]*model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record list after")
	return nil
}
