package encrypt

import (
	"demo/model/config/file"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*file.Encrypt, *file.EncryptReq, *file.EncryptRsp]
}

func (e *Creator) Create(ctx *types.ServiceContext, req *file.EncryptReq) (rsp *file.EncryptRsp, err error) {
	log := e.WithContext(ctx, ctx.Phase())
	log.Info("encrypt create")
	return rsp, nil
}
