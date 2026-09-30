package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Ping is the smallest model there is: an action with no table behind it,
// answered by service code, open to anyone. GET /api/pings.
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
		Records int64  `json:"records"` // the record count the record_count component keeps
	}
)
