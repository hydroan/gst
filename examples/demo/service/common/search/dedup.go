package search

import (
	"demo/model/common"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Dedup struct {
	service.Base[*common.Search, *common.SearchDedupReq, *common.SearchDedupRsp]
}

func (d *Dedup) Create(ctx *types.ServiceContext, req *common.SearchDedupReq) (rsp *common.SearchDedupRsp, err error) {
	log := d.WithServiceContext(ctx, ctx.GetPhase())
	log.Info("search: dedup")
	return rsp, nil
}
