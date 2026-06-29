package modeliamaccount

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type ChangePassword struct {
	model.Empty
}
type ChangePasswordReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type ChangePasswordRsp struct {
	Msg string `json:"msg,omitempty"`
}

func (ChangePassword) Design() {
	Route("/iam/change-password", func() {
		Create(func() {
			Service()
			Flatten()
			Filename("change_password.go")
			Payload[*ChangePasswordReq]()
			Result[*ChangePasswordRsp]()
		})
	})
}
