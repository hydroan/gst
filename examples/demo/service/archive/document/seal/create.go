package seal

import (
	"demo/model/archive/document"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*document.Seal, *document.SealReq, *document.SealRsp]
}

func (s *Creator) Create(ctx *gst.ServiceContext, req *document.SealReq) (rsp *document.SealRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())
	log.Info("seal create")
	return rsp, nil
}
