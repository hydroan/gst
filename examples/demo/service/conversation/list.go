package conversation

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*model.Conversation, *model.Conversation, *model.Conversation]
}

func (c *Lister) List(ctx *types.ServiceContext, req *model.Conversation) (rsp *model.Conversation, err error) {
	log := c.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("conversation list")
	return rsp, nil
}

func (c *Lister) ListBefore(ctx *types.ServiceContext, conversations *[]*model.Conversation) error {
	log := c.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("conversation list before")
	return nil
}

func (c *Lister) ListAfter(ctx *types.ServiceContext, conversations *[]*model.Conversation) error {
	log := c.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("conversation list after")
	return nil
}

func (c *Lister) Filter(ctx *types.ServiceContext, conversation *model.Conversation) *model.Conversation {
	return conversation
}

func (c *Lister) FilterRaw(ctx *types.ServiceContext) string {
	return ""
}
