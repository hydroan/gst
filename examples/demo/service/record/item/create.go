package item

import (
	"demo/model/record"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*record.Item, *record.Item, *record.Item]
}

func (m *Creator) Create(ctx *types.ServiceContext, req *record.Item) (rsp *record.Item, err error) {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("item create")
	return rsp, nil
}

func (m *Creator) CreateBefore(ctx *types.ServiceContext, item *record.Item) error {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("item create before")
	return nil
}

func (m *Creator) CreateAfter(ctx *types.ServiceContext, item *record.Item) error {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("item create after")
	return nil
}
