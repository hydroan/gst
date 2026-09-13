package file

import (
	"demo/model/config"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*config.File, *config.File, *config.File]
}

func (f *Lister) List(ctx *gst.ServiceContext, req *config.File) (rsp *config.File, err error) {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file list")
	return rsp, nil
}

func (f *Lister) ListBefore(ctx *gst.ServiceContext, files *[]*config.File) error {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file list before")
	return nil
}

func (f *Lister) ListAfter(ctx *gst.ServiceContext, files *[]*config.File) error {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file list after")
	return nil
}
