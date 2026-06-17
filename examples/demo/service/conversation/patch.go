package conversation

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Patcher struct {
	service.Base[*model.Conversation, *model.Conversation, *model.Conversation]
}

func (c *Patcher) Patch(ctx *types.ServiceContext, req *model.Conversation) (rsp *model.Conversation, err error) {
	log := c.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("conversation patch")
	return rsp, nil
}

func (c *Patcher) PatchBefore(ctx *types.ServiceContext, conversation *model.Conversation) error {
	log := c.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("conversation patch before")
	return nil
}

func (c *Patcher) PatchAfter(ctx *types.ServiceContext, conversation *model.Conversation) error {
	log := c.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("conversation patch after")
	return nil
}
