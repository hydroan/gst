package message

import (
	"demo/model/conversation"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*conversation.Message, *conversation.Message, *conversation.Message]
}

func (m *Creator) Create(ctx *types.ServiceContext, req *conversation.Message) (rsp *conversation.Message, err error) {
	log := m.WithContext(ctx, ctx.GetPhase())
	log.Info("message create")
	return rsp, nil
}

func (m *Creator) CreateBefore(ctx *types.ServiceContext, message *conversation.Message) error {
	log := m.WithContext(ctx, ctx.GetPhase())
	log.Info("message create before")
	return nil
}

func (m *Creator) CreateAfter(ctx *types.ServiceContext, message *conversation.Message) error {
	log := m.WithContext(ctx, ctx.GetPhase())
	log.Info("message create after")
	return nil
}
