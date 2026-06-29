package modeliamaccount

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Logout struct {
	model.Empty
}

type LogoutReq struct {
	model.Empty
}

type LogoutRsp struct {
	Msg string `json:"msg,omitempty"`
}

func (Logout) Design() {
	Route("/logout", func() {
		Create(func() {
			Service()
			Flatten()
			Filename("logout.go")
			Payload[*LogoutReq]()
			Result[*LogoutRsp]()
		})
	})
}
