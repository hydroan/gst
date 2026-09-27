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

// RebuildReq says how long the rebuild takes: long enough to send a second
// request while it runs and see that one refused. InTransaction asks for the
// lock to be taken from inside a transaction, which the framework refuses.
type RebuildReq struct {
	Seconds       int  `json:"seconds" pb:"1"`
	InTransaction bool `json:"in_transaction" pb:"2"`
}

// RebuildRsp reports which replica ran the rebuild and for how long.
type RebuildRsp struct {
	Replica string `json:"replica" pb:"1"`
	Seconds int    `json:"seconds" pb:"2"`
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
