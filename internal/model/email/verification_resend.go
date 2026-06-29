package modelemail

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// VerificationResendReq is the payload for POST /api/iam/email/verification-resend.
// It identifies the email address that should receive a new verification message.
type VerificationResendReq struct {
	Email string `json:"email" validate:"required,email"`
}

// VerificationResendRsp is the response for POST /api/iam/email/verification-resend.
// It returns the resend result message for the verification flow.
type VerificationResendRsp struct {
	Msg string `json:"msg,omitempty"`
}

type VerificationResend struct {
	model.Empty
}

func (VerificationResend) Design() {
	Route("/iam/email/verification-resend", func() {
		Create(func() {
			Service()
			Flatten()
			Public()
			Filename("verification_resend.go")
			Payload[*VerificationResendReq]()
			Result[*VerificationResendRsp]()
		})
	})
}
