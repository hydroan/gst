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

func (Cached) Design() {
	GRPC()

	Route("/caches", func() {
		Create(func() {
			Service()
			Payload[*CachedReq]()
			Result[*CachedCreateRsp]()
		})
		Get(func() {
			Service()
			Result[*CachedGetRsp]()
		})
		Delete(func() {
			Service()
			Result[*CachedDeleteRsp]()
		})
	})

	// LoadCached takes a stream of entries and answers once, with the count:
	// a client stream. ExchangeCached takes keys and answers each with what
	// this replica holds, for as long as the client keeps asking: a stream
	// both ways.
	Route("caches/load", func() {
		Stream(func() {
			Service("load")
			StreamingPayload[*CachedReq]()
			Result[*CachedLoadRsp]()
		})
	})
	Route("caches/exchange", func() {
		Stream(func() {
			Service("exchange")
			StreamingPayload[*CachedKeyReq]()
			StreamingResult[*CachedExchangeRsp]()
		})
	})
}

type (
	// CachedReq is the entry to write.
	CachedReq struct {
		Key   string `json:"key" pb:"1"`   // what the entry is filed under
		Value string `json:"value" pb:"2"` // what every other replica must end up holding for it
	}

	// CachedKeyReq names an entry to look up.
	CachedKeyReq struct {
		Key string `json:"key" pb:"1"`
	}

	// CachedLoadRsp reports what a load stream did.
	CachedLoadRsp struct {
		Replica string `json:"replica" pb:"1"` // the replica that took the entries in
		Count   int64  `json:"count" pb:"2"`   // how many entries the stream wrote
	}

	// CachedCreateRsp reports which replica wrote the entry and what it wrote.
	CachedCreateRsp struct {
		Replica string `json:"replica" pb:"1"`
		Key     string `json:"key" pb:"2"`
		Value   string `json:"value" pb:"3"`
		Found   bool   `json:"found" pb:"4"`
	}

	// CachedGetRsp reports which replica answered and what its own store
	// holds for the key, so a client reading every replica in turn sees the
	// propagation.
	CachedGetRsp struct {
		Replica string `json:"replica" pb:"1"`
		Key     string `json:"key" pb:"2"`
		Value   string `json:"value" pb:"3"`
		Found   bool   `json:"found" pb:"4"`
	}

	// CachedDeleteRsp reports which replica removed the entry.
	CachedDeleteRsp struct {
		Replica string `json:"replica" pb:"1"`
		Key     string `json:"key" pb:"2"`
	}

	// CachedExchangeRsp answers one key of an exchange stream: which replica
	// answered and what its own store holds for the key.
	CachedExchangeRsp struct {
		Replica string `json:"replica" pb:"1"`
		Key     string `json:"key" pb:"2"`
		Value   string `json:"value" pb:"3"`
		Found   bool   `json:"found" pb:"4"`
	}
)
