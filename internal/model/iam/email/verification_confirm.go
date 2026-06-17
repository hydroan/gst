package modeliamemail

// VerificationConfirmReq is the payload for POST /api/iam/email/verification-confirm.
// It carries the verification token from the email link or client-side confirmation flow.
type VerificationConfirmReq struct {
	Token string `json:"token" validate:"required"`
}

// VerificationConfirmRsp is the response for POST /api/iam/email/verification-confirm.
// It indicates whether the email has been verified successfully.
type VerificationConfirmRsp struct {
	Verified bool   `json:"verified"`
	Msg      string `json:"msg,omitempty"`
}
