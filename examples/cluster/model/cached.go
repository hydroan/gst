package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Cached is an entry of the replicated cache every replica keeps: written on
// the replica that answers the request, read back from the local store of
// whichever replica answers the next one. It is what makes the propagation
// between replicas observable from outside — the store itself is process
// memory, with no shared tier behind it.
type Cached struct {
	model.Empty
}

// CachedReq is the entry to write: the key it is filed under and the value
// every other replica must end up holding for it.
type CachedReq struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// CachedRsp reports which replica answered and what its own store holds for
// the key, so a client reading every replica in turn sees the propagation.
type CachedRsp struct {
	Replica string `json:"replica"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Found   bool   `json:"found"`
}

func (Cached) Design() {
	Route("/caches", func() {
		Create(func() {
			Service()
			Payload[*CachedReq]()
			Result[*CachedRsp]()
		})
		Get(func() {
			Service()
			Result[*CachedRsp]()
		})
		Delete(func() {
			Service()
			Result[*CachedRsp]()
		})
	})
}
