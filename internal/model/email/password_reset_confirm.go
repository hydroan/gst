package modelemail

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// PasswordResetConfirmReq is the payload for POST /api/iam/email/password-reset-confirm.
// It carries the reset token and the new password from the password reset flow.
type PasswordResetConfirmReq struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=6"`
}

// PasswordResetConfirmRsp is the response for POST /api/iam/email/password-reset-confirm.
// It indicates whether the password has been reset successfully.
type PasswordResetConfirmRsp struct {
	Reset bool   `json:"reset"`
	Msg   string `json:"msg,omitempty"`
}

// PasswordResetConfirm is the model for POST /api/iam/email/password-reset-confirm.
// It completes an email-based password reset flow with the issued reset token.
type PasswordResetConfirm struct {
	model.Empty
}

func (PasswordResetConfirm) Design() {
	Route("/iam/email/password-reset-confirm", func() {
		Create(func() {
			Service(true)
			Flatten()
			Public()
			Filename("password_reset_confirm.go")
			Payload[*PasswordResetConfirmReq]()
			Result[*PasswordResetConfirmRsp]()
		})
	})
}
