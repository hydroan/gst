package login

import (
	"demo/model/auth"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Login struct {
	service.Base[*auth.Login, *auth.Login, *auth.LoginRsp]
}

func (l *Login) List(ctx *types.ServiceContext, req *auth.Login) (rsp *auth.LoginRsp, err error) {
	log := l.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("login: login")
	return rsp, nil
}
