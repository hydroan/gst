package record

import (
	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*model.Record, *model.Record, *model.Record]
}

func (c *Creator) Create(ctx *gst.ServiceContext, req *model.Record) (rsp *model.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}

func (c *Creator) CreateBefore(ctx *gst.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record create before")
	return nil
}

func (c *Creator) CreateAfter(ctx *gst.ServiceContext, record *model.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record create after")
	return nil
}
