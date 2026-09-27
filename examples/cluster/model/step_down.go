package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// StepDown is the request that ends the leadership on the replica that takes
// it: the leader work returns a failure of its own instead of running until
// its context ends, which is how a deployment sees what the framework does
// with work that gives the name back early.
type StepDown struct {
	model.Empty
}

// StepDownRsp reports which replica took the request, and whether that
// replica had leader work to end.
type StepDownRsp struct {
	Replica string `json:"replica" pb:"1"`
	Asked   bool   `json:"asked" pb:"2"`
}

func (StepDown) Design() {
	GRPC()

	Route("/step-downs", func() {
		Create(func() {
			Service()
			Result[*StepDownRsp]()
		})
	})
}
