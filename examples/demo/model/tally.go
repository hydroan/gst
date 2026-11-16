package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Tally keeps a running total for a client: the client sends amounts for as
// long as it likes, and each is answered with the total the connection has
// carried so far. It is a stream both ways, over WebSocket alone: a Stream
// action is served over HTTP as a WebSocket connection, the route opened
// with a GET and every message after that one text frame holding a JSON
// document, and a model declaring no GRPC(), like this one, serves its
// streams over HTTP alone. GET /api/tallies.
type Tally struct {
	model.Empty
}

func (Tally) Design() {
	Stream(func() {
		Public()
		Service("sum")
		StreamingPayload[*TallySumReq]()
		StreamingResult[*TallySumRsp]()
	})
}

type (
	// TallySumReq is an amount to add to the total.
	TallySumReq struct {
		Amount int64 `json:"amount"`
	}

	// TallySumRsp is the total once an amount was added.
	TallySumRsp struct {
		Total int64 `json:"total"`
	}
)
