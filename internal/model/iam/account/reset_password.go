package modeliamaccount

type ResetPasswordReq struct {
	UserID      string `json:"user_id" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=6"`
}

type ResetPasswordRsp struct {
	Msg string `json:"msg,omitempty"`
}
