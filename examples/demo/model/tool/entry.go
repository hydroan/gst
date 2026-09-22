package tool

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Entry is a non-database action model.
type Entry struct {
	model.Empty
}

// EntryPair is one key/value pair submitted for merging.
type EntryPair struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// EntryMergeReq is the request for merging entries.
type EntryMergeReq struct {
	Entries []EntryPair `json:"entries"`
}

// EntryMergeRsp is the response returned after merging.
type EntryMergeRsp struct {
	Entries []EntryPair `json:"entries"`
}

func (Entry) Design() {
	Route("/entries/merge", func() {
		Create(func() {
			Filename("merge")
			Service()
			Payload[*EntryMergeReq]()
			Result[*EntryMergeRsp]()
		})
	})
}
