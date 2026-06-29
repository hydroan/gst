package modelemail

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// VerificationRequestReq is the payload for POST /api/iam/email/verification-request.
// It accepts the email address that should receive the verification message.
type VerificationRequestReq struct {
	Email string `json:"email" validate:"required,email"`
}

// VerificationRequestRsp is the response for POST /api/iam/email/verification-request.
// It returns the delivery result message for the verification request.
type VerificationRequestRsp struct {
	Msg string `json:"msg,omitempty"`
}

type VerificationRequest struct {
	model.Empty
}

func (VerificationRequest) Design() {
	Route("/iam/email/verification-request", func() {
		Create(func() {
			Service(true)
			Flatten()
			Public()
			Filename("verification_request.go")
			Payload[*VerificationRequestReq]()
			Result[*VerificationRequestRsp]()
		})
	})
}
