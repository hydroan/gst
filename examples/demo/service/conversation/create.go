package conversation

import (
	"demo/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*model.Conversation, *model.Conversation, *model.Conversation]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *model.Conversation) (rsp *model.Conversation, err error) {
	log := c.WithContext(ctx, ctx.GetPhase())
	log.Info("conversation create")
	return rsp, nil
}

func (c *Creator) CreateBefore(ctx *types.ServiceContext, conversation *model.Conversation) error {
	log := c.WithContext(ctx, ctx.GetPhase())
	log.Info("conversation create before")
	return nil
}

func (c *Creator) CreateAfter(ctx *types.ServiceContext, conversation *model.Conversation) error {
	log := c.WithContext(ctx, ctx.GetPhase())
	log.Info("conversation create after")
	return nil
}
