package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Rebuild is the action a client triggers: work that must not run twice at
// once across the deployment. The service runs it under the "rebuild" lock.
type Rebuild struct {
	model.Empty
}

func (Rebuild) Design() {
	GRPC()

	Route("/rebuilds", func() {
		Create(func() {
			Service()
			Payload[*RebuildReq]()
			Result[*RebuildRsp]()
		})
	})
}

type (
	// RebuildReq says how the rebuild runs.
	RebuildReq struct {
		Seconds       int  `json:"seconds" pb:"1"`        // how long the rebuild takes: long enough to send a second request while it runs and see that one refused
		InTransaction bool `json:"in_transaction" pb:"2"` // asks for the lock to be taken from inside a transaction, which the framework refuses
	}

	// RebuildRsp reports which replica ran the rebuild and for how long.
	RebuildRsp struct {
		Replica string `json:"replica" pb:"1"`
		Seconds int    `json:"seconds" pb:"2"`
	}
)
