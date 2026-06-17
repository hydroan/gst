package modeltwofa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPConfirm 确认绑定 TOTP 设备
type TOTPConfirm struct {
	model.Empty
}
type TOTPConfirmReq struct {
	Secret     string `json:"secret" validate:"required"`
	Code       string `json:"code" validate:"required,len=6"` // 6-digit TOTP code to confirm
	DeviceName string `json:"device_name" validate:"required,max=100"`
}

type TOTPConfirmRsp struct {
	DeviceID    string   `json:"device_id,omitempty"`
	Message     string   `json:"message,omitempty"`
	BackupCodes []string `json:"backup_codes,omitempty"` // 8-digit backup codes
}

func (TOTPConfirm) Design() {
	Route("2fa/totp/confirm", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Payload[*TOTPConfirmReq]()
			Result[*TOTPConfirmRsp]()
		})
	})
}
