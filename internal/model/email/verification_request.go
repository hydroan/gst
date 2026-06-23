package modelemail

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
