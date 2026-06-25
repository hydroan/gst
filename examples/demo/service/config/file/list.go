package file

import (
	"demo/model/config"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*config.File, *config.File, *config.File]
}

func (f *Lister) List(ctx *types.ServiceContext, req *config.File) (rsp *config.File, err error) {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file list")
	return rsp, nil
}

func (f *Lister) ListBefore(ctx *types.ServiceContext, files *[]*config.File) error {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file list before")
	return nil
}

func (f *Lister) ListAfter(ctx *types.ServiceContext, files *[]*config.File) error {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file list after")
	return nil
}

func (f *Lister) Filter(ctx *types.ServiceContext, file *config.File) *config.File {
	return file
}

func (f *Lister) FilterRaw(ctx *types.ServiceContext) string {
	return ""
}
