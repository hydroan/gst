package modelemail

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// ChangeConfirmReq is the payload for POST /api/iam/email/change-confirm.
// It carries the confirmation token from the email change link or confirmation page.
type ChangeConfirmReq struct {
	Token string `json:"token" validate:"required"`
}

// ChangeConfirmRsp is the response for POST /api/iam/email/change-confirm.
// It indicates whether the pending email change has been confirmed successfully.
type ChangeConfirmRsp struct {
	Changed bool   `json:"changed"`
	Msg     string `json:"msg,omitempty"`
}

// ChangeConfirm is the model for POST /api/iam/email/change-confirm.
// It completes the pending email change flow with the issued confirmation token.
type ChangeConfirm struct {
	model.Empty
}

func (ChangeConfirm) Design() {
	Route("/iam/email/change-confirm", func() {
		Create(func() {
			Service()
			Flatten()
			Filename("change_confirm.go")
			Payload[*ChangeConfirmReq]()
			Result[*ChangeConfirmRsp]()
		})
	})
}
