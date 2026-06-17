package modeltwofa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPVerify 验证 TOTP 代码
type TOTPVerify struct {
	model.Empty
}

type TOTPVerifyReq struct {
	Code     string `json:"code" validate:"required,len=6"` // 6-digit TOTP code
	DeviceID string `json:"device_id,omitempty"`            // Optional: specific device ID
	IsBackup bool   `json:"is_backup,omitempty"`            // Whether this is a backup code
}

type TOTPVerifyRsp struct {
	Valid   bool   `json:"valid,omitempty"`
	Message string `json:"message,omitempty"`
}

func (TOTPVerify) Design() {
	Route("2fa/totp/verify", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Public(false) // Requires authentication
			Payload[*TOTPVerifyReq]()
			Result[*TOTPVerifyRsp]()
		})
	})
}
