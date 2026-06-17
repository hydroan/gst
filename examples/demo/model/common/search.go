package common

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Search demonstrates a non-database utility action.
type Search struct {
	model.Empty
}

// SearchSource is one candidate source returned by an external search provider.
type SearchSource struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// SearchDedupReq is the request for deduplicating search sources.
type SearchDedupReq struct {
	Sources []SearchSource `json:"sources"`
}

// SearchDedupRsp is the response returned after source deduplication.
type SearchDedupRsp struct {
	Sources []SearchSource `json:"sources"`
}

func (Search) Design() {
	Route("/search-sources/dedup", func() {
		Create(func() {
			Filename("dedup")
			Service(true)
			Payload[*SearchDedupReq]()
			Result[*SearchDedupRsp]()
		})
	})
}
