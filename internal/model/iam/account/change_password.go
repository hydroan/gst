package modeliamaccount

type ChangePasswordReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type ChangePasswordRsp struct {
	Msg string `json:"msg,omitempty"`
}
