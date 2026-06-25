package service

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type user struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (u *user) Create(ctx *types.ServiceContext, req *model.User) (rsp *model.User, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create")
	return rsp, nil
}

func (u *user) CreateBefore(ctx *types.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create before")
	return nil
}

func (u *user) CreateAfter(ctx *types.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create after")
	return nil
}
