package modeliamaccount

type ChangePasswordReq struct {
	OldPassword string `json:"old_password" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=6"`
}

type ChangePasswordRsp struct {
	Msg string `json:"msg,omitempty"`
}
