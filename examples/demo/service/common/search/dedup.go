package search

import (
	"demo/model/common"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Dedup struct {
	service.Base[*common.Search, *common.SearchDedupReq, *common.SearchDedupRsp]
}

func (d *Dedup) Create(ctx *gst.ServiceContext, req *common.SearchDedupReq) (rsp *common.SearchDedupRsp, err error) {
	log := d.WithContext(ctx, ctx.Phase())
	log.Info("search: dedup")
	return rsp, nil
}
