package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// CacheEntry is an entry of the replicated cache every replica keeps:
// written on the replica that answers the request, read back from the local
// store of whichever replica answers the next one. It is what makes the
// propagation between replicas observable from outside — the store itself is
// process memory, with no shared tier behind it.
type CacheEntry struct {
	model.Empty
}

func (CacheEntry) Design() {
	GRPC()

	Route("/cache_entries", func() {
		Create(func() {
			Service()
			Payload[*CacheEntryReq]()
			Result[*CacheEntryCreateRsp]()
		})
		Get(func() {
			Service()
			Result[*CacheEntryGetRsp]()
		})
		Delete(func() {
			Service()
			Result[*CacheEntryDeleteRsp]()
		})
	})

	// LoadCacheEntry takes a stream of entries and answers once, with the
	// count: a client stream. ExchangeCacheEntry takes keys and answers each
	// with what this replica holds, for as long as the client keeps asking: a
	// stream both ways.
	Route("cache_entries/load", func() {
		Stream(func() {
			Service("load")
			StreamingPayload[*CacheEntryLoadReq]()
			Result[*CacheEntryLoadRsp]()
		})
	})
	Route("cache_entries/exchange", func() {
		Stream(func() {
			Service("exchange")
			StreamingPayload[*CacheEntryKeyReq]()
			StreamingResult[*CacheEntryExchangeRsp]()
		})
	})
}

type (
	// CacheEntryReq is the entry to write.
	CacheEntryReq struct {
		Key   string `json:"key" pb:"1"`   // what the entry is filed under
		Value string `json:"value" pb:"2"` // what every other replica must end up holding for it
	}

	// CacheEntryLoadReq is an entry the load stream writes: the same entry as
	// CacheEntryReq, under the stream's own name.
	CacheEntryLoadReq = CacheEntryReq

	// CacheEntryKeyReq names an entry to look up.
	CacheEntryKeyReq struct {
		Key string `json:"key" pb:"1"`
	}

	// CacheEntryLoadRsp reports what a load stream did.
	CacheEntryLoadRsp struct {
		Replica string `json:"replica" pb:"1"` // the replica that took the entries in
		Count   int64  `json:"count" pb:"2"`   // how many entries the stream wrote
	}

	// CacheEntryCreateRsp reports which replica wrote the entry and what it
	// wrote.
	CacheEntryCreateRsp struct {
		Replica string `json:"replica" pb:"1"`
		Key     string `json:"key" pb:"2"`
		Value   string `json:"value" pb:"3"`
		Found   bool   `json:"found" pb:"4"`
	}

	// CacheEntryGetRsp reports which replica answered and what its own store
	// holds for the key, so a client reading every replica in turn sees the
	// propagation.
	CacheEntryGetRsp struct {
		Replica string `json:"replica" pb:"1"`
		Key     string `json:"key" pb:"2"`
		Value   string `json:"value" pb:"3"`
		Found   bool   `json:"found" pb:"4"`
	}

	// CacheEntryDeleteRsp reports which replica removed the entry.
	CacheEntryDeleteRsp struct {
		Replica string `json:"replica" pb:"1"`
		Key     string `json:"key" pb:"2"`
	}

	// CacheEntryExchangeRsp answers one key of an exchange stream: which replica
	// answered and what its own store holds for the key.
	CacheEntryExchangeRsp struct {
		Replica string `json:"replica" pb:"1"`
		Key     string `json:"key" pb:"2"`
		Value   string `json:"value" pb:"3"`
		Found   bool   `json:"found" pb:"4"`
	}
)
