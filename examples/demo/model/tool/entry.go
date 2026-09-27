package tool

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Entry is a utility action with no table: merging key/value pairs, at
// POST /api/entries/merge. Filename names the service file after the action
// rather than the phase, and Flatten puts it in the package of the model's
// directory, service/tool/merge.go in package tool, instead of a package of
// the model file's own.
type Entry struct {
	model.Empty
}

// EntryPair is one key/value pair.
type EntryPair struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// EntryMergeReq is the pairs to merge.
type EntryMergeReq struct {
	Entries []EntryPair `json:"entries"`
}

// EntryMergeRsp is the pairs merged by key.
type EntryMergeRsp struct {
	Entries []EntryPair `json:"entries"`
}

func (Entry) Design() {
	Route("entries/merge", func() {
		Create(func() {
			Flatten()
			Service("merge")
			Payload[*EntryMergeReq]()
			Result[*EntryMergeRsp]()
		})
	})
}
