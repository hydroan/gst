package modelemail

import "github.com/hydroan/gst/model"

// ChangeResend is the model for POST /api/iam/email/change-resend.
// It resends a confirmation message for a pending email change request.
type ChangeResend struct {
	model.Empty
}

// ChangeResendReq is the payload for POST /api/iam/email/change-resend.
// It identifies the pending target email address that should receive a new confirmation message.
type ChangeResendReq struct {
	NewEmail string `json:"new_email" validate:"required,email"`
}

// ChangeResendRsp is the response for POST /api/iam/email/change-resend.
// It returns the resend result message for the email change flow.
type ChangeResendRsp struct {
	Msg string `json:"msg,omitempty"`
}
