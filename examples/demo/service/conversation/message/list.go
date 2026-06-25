package message

import (
	"demo/model/conversation"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*conversation.Message, *conversation.Message, *conversation.Message]
}

func (m *Lister) List(ctx *types.ServiceContext, req *conversation.Message) (rsp *conversation.Message, err error) {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("message list")
	return rsp, nil
}

func (m *Lister) ListBefore(ctx *types.ServiceContext, messages *[]*conversation.Message) error {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("message list before")
	return nil
}

func (m *Lister) ListAfter(ctx *types.ServiceContext, messages *[]*conversation.Message) error {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("message list after")
	return nil
}

func (m *Lister) Filter(ctx *types.ServiceContext, message *conversation.Message) *conversation.Message {
	return message
}

func (m *Lister) FilterRaw(ctx *types.ServiceContext) string {
	return ""
}
