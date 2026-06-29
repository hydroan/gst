package modeliamaccount

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Signup struct {
	model.Empty
}

type SignupReq struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RePassword string `json:"re_password"`
	Email      string `json:"email,omitempty"`
}

type SignupRsp struct {
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Message  string `json:"message,omitempty"`
}

func (Signup) Design() {
	Route("/signup", func() {
		Create(func() {
			Service()
			Flatten()
			Public()
			Filename("signup.go")
			Payload[*SignupReq]()
			Result[*SignupRsp]()
		})
	})
}
