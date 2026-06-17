package modeliamaccount

import modeliamuser "github.com/hydroan/gst/internal/model/iam/user"

type AccountStatusReq struct {
	UserID string                  `json:"user_id" validate:"required"`
	Status modeliamuser.UserStatus `json:"status" validate:"required"`
}

type AccountStatusRsp struct {
	Msg string `json:"msg,omitempty"`
}
