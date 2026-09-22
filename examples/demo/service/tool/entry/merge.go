package entry

import (
	"demo/model/tool"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Merge struct {
	service.Base[*tool.Entry, *tool.EntryMergeReq, *tool.EntryMergeRsp]
}

func (m *Merge) Create(ctx *gst.ServiceContext, req *tool.EntryMergeReq) (rsp *tool.EntryMergeRsp, err error) {
	log := m.WithContext(ctx, ctx.Phase())
	log.Info("entry: merge")

	return rsp, nil
}
