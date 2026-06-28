package modeliamuser

type UserStatusPatchReq struct {
	Status UserStatus `json:"status" validate:"required"`
}

type UserStatusPatchRsp struct {
	Msg string `json:"msg,omitempty"`
}
