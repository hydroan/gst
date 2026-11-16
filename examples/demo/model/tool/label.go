package tool

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Label is the utility that works on labels, the key/value pairs things
// are tagged with: it merges sets of them by key, at POST
// /api/labels/merge. It is an action with no table: Service("merge") names
// the service file after the action rather than the phase, and Flatten puts
// it in the package of the model's directory, service/tool/merge.go in
// package tool, instead of a package of the model file's own.
type Label struct {
	model.Empty
}

func (Label) Design() {
	Route("labels/merge", func() {
		Create(func() {
			Flatten()
			Service("merge")
			Payload[*LabelMergeReq]()
			Result[*LabelMergeRsp]()
		})
	})
}

type (
	// LabelPair is one label, a key and its value.
	LabelPair struct {
		Key   string `json:"key"`
		Value string `json:"value,omitempty"`
	}

	// LabelMergeReq is the labels to merge.
	LabelMergeReq struct {
		Labels []LabelPair `json:"labels"`
	}

	// LabelMergeRsp is the labels merged by key.
	LabelMergeRsp struct {
		Labels []LabelPair `json:"labels"`
	}
)
