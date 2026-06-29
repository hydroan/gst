package modeliamaccount

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type ResetPassword struct {
	model.Empty
}
type ResetPasswordReq struct {
	UserID      string `json:"user_id"`
	NewPassword string `json:"new_password"`
}

type ResetPasswordRsp struct {
	Msg string `json:"msg,omitempty"`
}

func (ResetPassword) Design() {
	Route("/iam/reset-password", func() {
		Create(func() {
			Service()
			Flatten()
			Filename("reset_password.go")
			Payload[*ResetPasswordReq]()
			Result[*ResetPasswordRsp]()
		})
	})
}
