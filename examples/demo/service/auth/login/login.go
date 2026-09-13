package login

import (
	"demo/model/auth"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

type Login struct {
	service.Base[*auth.Login, *model.Empty, *auth.LoginRsp]
}

func (l *Login) List(ctx *gst.ServiceContext, req *model.Empty) (rsp *auth.LoginRsp, err error) {
	log := l.WithContext(ctx, ctx.Phase())
	log.Info("login: login")
	return rsp, nil
}
