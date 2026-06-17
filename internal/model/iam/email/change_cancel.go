package modeliamemail

import "github.com/hydroan/gst/model"

// ChangeCancel is the model for POST /api/iam/email/change-cancel.
// It cancels a pending email change flow with the issued cancellation token.
type ChangeCancel struct {
	model.Empty
}

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
