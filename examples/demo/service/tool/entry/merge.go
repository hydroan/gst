package entry

import (
	"demo/model/tool"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Merge struct {
	service.Base[*tool.Entry, *tool.EntryMergeReq, *tool.EntryMergeRsp]
}

// Create merges the submitted entries by key: a later value for a key replaces
// the earlier one, and every key keeps the position it first appeared at.
func (m *Merge) Create(ctx *gst.ServiceContext, req *tool.EntryMergeReq) (rsp *tool.EntryMergeRsp, err error) {
	index := make(map[string]int, len(req.Entries))
	rsp = &tool.EntryMergeRsp{}
	for _, pair := range req.Entries {
		if i, ok := index[pair.Key]; ok {
			rsp.Entries[i] = pair // a later value for the same key wins
			continue
		}
		index[pair.Key] = len(rsp.Entries)
		rsp.Entries = append(rsp.Entries, pair)
	}
	return rsp, nil
}
