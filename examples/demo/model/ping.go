package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Ping answers whether the service is up. It is the smallest model there
// is: an action with no table behind it, answered by service code, open to
// anyone. GET /api/pings.
type Ping struct {
	model.Empty
}

func (Ping) Design() {
	List(func() {
		Public()
		Service()
		Result[*PingRsp]()
	})
}

type (
	// PingRsp is what a ping answers.
	PingRsp struct {
		Msg     string `json:"msg"`     // a word
		Tickets int64  `json:"tickets"` // the ticket count the count_tickets job keeps in dao
	}
)
