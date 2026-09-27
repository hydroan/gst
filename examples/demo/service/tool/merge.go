package tool

import (
	"demo/model/tool"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Merge answers POST /api/entries/merge: a custom action with request and
// response types of its own, in the package of the model's directory
// because the action declares Flatten.
type Merge struct {
	service.Base[*tool.Entry, *tool.EntryMergeReq, *tool.EntryMergeRsp]
}

// Create merges the entries by key: a later value for a key replaces the
// earlier one, and every key keeps the position it first appeared at.
func (m *Merge) Create(_ *gst.ServiceContext, req *tool.EntryMergeReq) (*tool.EntryMergeRsp, error) {
	index := make(map[string]int, len(req.Entries))
	rsp := &tool.EntryMergeRsp{Entries: make([]tool.EntryPair, 0, len(req.Entries))}
	for _, pair := range req.Entries {
		if i, ok := index[pair.Key]; ok {
			rsp.Entries[i] = pair
			continue
		}
		index[pair.Key] = len(rsp.Entries)
		rsp.Entries = append(rsp.Entries, pair)
	}
	return rsp, nil
}
