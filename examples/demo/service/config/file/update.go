package file

import (
	"demo/model/config"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Updater struct {
	service.Base[*config.File, *config.File, *config.File]
}

func (f *Updater) Update(ctx *types.ServiceContext, req *config.File) (rsp *config.File, err error) {
	log := f.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("file update")
	return rsp, nil
}

func (f *Updater) UpdateBefore(ctx *types.ServiceContext, file *config.File) error {
	log := f.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("file update before")
	return nil
}

func (f *Updater) UpdateAfter(ctx *types.ServiceContext, file *config.File) error {
	log := f.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("file update after")
	return nil
}
