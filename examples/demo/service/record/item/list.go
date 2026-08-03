package item

import (
	"demo/model/record"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*record.Item, *record.Item, *record.Item]
}

func (m *Lister) List(ctx *types.ServiceContext, req *record.Item) (rsp *record.Item, err error) {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("item list")
	return rsp, nil
}

func (m *Lister) ListBefore(ctx *types.ServiceContext, items *[]*record.Item) error {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("item list before")
	return nil
}

func (m *Lister) ListAfter(ctx *types.ServiceContext, items *[]*record.Item) error {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("item list after")
	return nil
}
