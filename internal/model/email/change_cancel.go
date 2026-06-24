package modelemail

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// ChangeCancelReq is the payload for POST /api/iam/email/change-cancel.
// It carries the cancellation token from the notification sent to the old email address.
type ChangeCancelReq struct {
	Token string `json:"token" validate:"required"`
}

// ChangeCancelRsp is the response for POST /api/iam/email/change-cancel.
// It indicates whether the pending email change has been canceled successfully.
type ChangeCancelRsp struct {
	Canceled bool   `json:"canceled"`
	Msg      string `json:"msg,omitempty"`
}

// ChangeCancel is the model for POST /api/iam/email/change-cancel.
// It cancels a pending email change flow with the issued cancellation token.
type ChangeCancel struct {
	model.Empty
}

func (ChangeCancel) Design() {
	Route("/iam/email/change-cancel", func() {
		Create(func() {
			Public(true)
			Service(true)
			Flatten()
			Filename("change_cancel.go")
			Payload[*ChangeCancelReq]()
			Result[*ChangeCancelRsp]()
		})
	})
}
