package modeliamemail

import "github.com/hydroan/gst/model"

// ChangeConfirm is the model for POST /api/iam/email/change-confirm.
// It completes the pending email change flow with the issued confirmation token.
type ChangeConfirm struct {
	model.Empty
}

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
