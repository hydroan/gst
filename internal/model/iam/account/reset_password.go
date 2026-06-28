package modeliamaccount

type ResetPasswordReq struct {
	UserID      string `json:"user_id"`
	NewPassword string `json:"new_password"`
}

type ResetPasswordRsp struct {
	Msg string `json:"msg,omitempty"`
}
