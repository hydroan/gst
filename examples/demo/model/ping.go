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

// PingRsp is what a ping answers: a word, and the record count the
// record_count component keeps.
type PingRsp struct {
	Msg     string `json:"msg"`
	Records int64  `json:"records"`
}

func (Ping) Design() {
	List(func() {
		Public()
		Service()
		Result[*PingRsp]()
	})
}
