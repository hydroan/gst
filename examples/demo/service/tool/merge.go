package tool

import (
	"demo/model/tool"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Merge answers POST /api/labels/merge: a custom action with request and
// response types of its own, in the package of the model's directory
// because the action declares Flatten.
type Merge struct {
	service.Base[*tool.Label, *tool.LabelMergeReq, *tool.LabelMergeRsp]
}

// Create merges the labels by key: a later value for a key replaces the
// earlier one, and every key keeps the position it first appeared at.
func (m *Merge) Create(_ *gst.ServiceContext, req *tool.LabelMergeReq) (*tool.LabelMergeRsp, error) {
	index := make(map[string]int, len(req.Labels))
	rsp := &tool.LabelMergeRsp{Labels: make([]tool.LabelPair, 0, len(req.Labels))}
	for _, pair := range req.Labels {
		if i, ok := index[pair.Key]; ok {
			rsp.Labels[i] = pair
			continue
		}
		index[pair.Key] = len(rsp.Labels)
		rsp.Labels = append(rsp.Labels, pair)
	}
	return rsp, nil
}
