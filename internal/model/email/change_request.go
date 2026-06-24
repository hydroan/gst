package modelemail

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// ChangeRequestReq is the payload for POST /api/iam/email/change-request.
// It carries the target email address and the current password for re-authentication.
type ChangeRequestReq struct {
	NewEmail        string `json:"new_email" validate:"required,email"`
	CurrentPassword string `json:"current_password" validate:"required"`
}

// ChangeRequestRsp is the response for POST /api/iam/email/change-request.
// It returns the request result message for the email change flow.
type ChangeRequestRsp struct {
	Msg string `json:"msg,omitempty"`
}

// ChangeRequest is the model for POST /api/iam/email/change-request.
// It starts a protected email change flow for the current authenticated user.
type ChangeRequest struct {
	model.Empty
}

func (ChangeRequest) Design() {
	Route("/iam/email/change-request", func() {
		Create(func() {
			Service(true)
			Flatten()
			Filename("change_request.go")
			Payload[*ChangeRequestReq]()
			Result[*ChangeRequestRsp]()
		})
	})
}
