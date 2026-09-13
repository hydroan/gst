package item

import (
	"demo/model/record"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type ManyDeleter struct {
	service.Base[*record.Item, *record.Item, *record.Item]
}

func (m *ManyDeleter) DeleteMany(ctx *gst.ServiceContext, req *record.Item) (rsp *record.Item, err error) {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("item delete many")
	return rsp, nil
}

func (m *ManyDeleter) DeleteManyBefore(ctx *gst.ServiceContext, items ...*record.Item) error {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("item delete many before")
	return nil
}

func (m *ManyDeleter) DeleteManyAfter(ctx *gst.ServiceContext, items ...*record.Item) error {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("item delete many after")
	return nil
}
