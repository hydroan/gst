package message

import (
	"demo/model/conversation"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type ManyDeleter struct {
	service.Base[*conversation.Message, *conversation.Message, *conversation.Message]
}

func (m *ManyDeleter) DeleteMany(ctx *types.ServiceContext, req *conversation.Message) (rsp *conversation.Message, err error) {
	log := m.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("message delete many")
	return rsp, nil
}

func (m *ManyDeleter) DeleteManyBefore(ctx *types.ServiceContext, messages ...*conversation.Message) error {
	log := m.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("message delete many before")
	return nil
}

func (m *ManyDeleter) DeleteManyAfter(ctx *types.ServiceContext, messages ...*conversation.Message) error {
	log := m.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("message delete many after")
	return nil
}
