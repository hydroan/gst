package file

import (
	"demo/model/config"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*config.File, *config.File, *config.File]
}

func (f *Creator) Create(ctx *types.ServiceContext, req *config.File) (rsp *config.File, err error) {
	log := f.WithContext(ctx, ctx.GetPhase())
	log.Info("file create")
	return rsp, nil
}

func (f *Creator) CreateBefore(ctx *types.ServiceContext, file *config.File) error {
	log := f.WithContext(ctx, ctx.GetPhase())
	log.Info("file create before")
	return nil
}

func (f *Creator) CreateAfter(ctx *types.ServiceContext, file *config.File) error {
	log := f.WithContext(ctx, ctx.GetPhase())
	log.Info("file create after")
	return nil
}
