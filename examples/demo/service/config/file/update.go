package file

import (
	"demo/model/config"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Updater struct {
	service.Base[*config.File, *config.File, *config.File]
}

func (f *Updater) Update(ctx *gst.ServiceContext, req *config.File) (rsp *config.File, err error) {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file update")
	return rsp, nil
}

func (f *Updater) UpdateBefore(ctx *gst.ServiceContext, file *config.File) error {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file update before")
	return nil
}

func (f *Updater) UpdateAfter(ctx *gst.ServiceContext, file *config.File) error {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file update after")
	return nil
}
