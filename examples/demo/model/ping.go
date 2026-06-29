package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Ping struct {
	model.Empty
}
type PingRsp struct {
	Msg string
}

func (Ping) Design() {
	dsl.List(func() {
		dsl.Public()
		dsl.Service(true)
		dsl.Result[*PingRsp]()
	})
}
