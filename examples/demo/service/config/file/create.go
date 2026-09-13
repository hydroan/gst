package file

import (
	"demo/model/config"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*config.File, *config.File, *config.File]
}

func (f *Creator) Create(ctx *gst.ServiceContext, req *config.File) (rsp *config.File, err error) {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file create")
	return rsp, nil
}

func (f *Creator) CreateBefore(ctx *gst.ServiceContext, file *config.File) error {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file create before")
	return nil
}

func (f *Creator) CreateAfter(ctx *gst.ServiceContext, file *config.File) error {
	log := f.WithContext(ctx, ctx.Phase())
	log.Info("file create after")
	return nil
}
