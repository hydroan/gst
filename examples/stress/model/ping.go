package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Ping answers GET /api/ping without a session: the request that does no
// work, which every other scenario is measured against.
type Ping struct {
	model.Empty
}

func (Ping) Design() {
	Route("ping", func() {
		List(func() {
			Public()
			Service("ping")
			Result[*PingRsp]()
		})
	})
}

// PingRsp is the answer of GET /api/ping.
type PingRsp struct {
	Msg string `json:"msg"`
}
