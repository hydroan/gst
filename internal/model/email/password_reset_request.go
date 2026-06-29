package modelemail

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// PasswordResetRequestReq is the payload for POST /api/iam/email/password-reset-request.
// It accepts the email address that should receive the password reset message.
type PasswordResetRequestReq struct {
	Email string `json:"email" validate:"required,email"`
}

// PasswordResetRequestRsp is the response for POST /api/iam/email/password-reset-request.
// It returns the request result message for the email password reset flow.
type PasswordResetRequestRsp struct {
	Msg string `json:"msg,omitempty"`
}

// PasswordResetRequest is the model for POST /api/iam/email/password-reset-request.
// It starts an email-based password reset flow for the target account.
type PasswordResetRequest struct {
	model.Empty
}

func (PasswordResetRequest) Design() {
	Route("/iam/email/password-reset-request", func() {
		Create(func() {
			Service()
			Flatten()
			Public()
			Filename("password_reset_request.go")
			Payload[*PasswordResetRequestReq]()
			Result[*PasswordResetRequestRsp]()
		})
	})
}
