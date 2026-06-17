package modeltwofa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPUnbind 解绑 TOTP 设备
type TOTPUnbind struct {
	model.Empty
}
type TOTPUnbindReq struct {
	DeviceID string `json:"device_id" validate:"required"` // 要解绑的设备ID
	Password string `json:"password,omitempty"`
	TOTPCode string `json:"totp_code,omitempty" validate:"len=6,numeric"` // TOTP验证码（可选，用于额外验证）
}

type TOTPUnbindRsp struct {
	Success     bool   `json:"success,omitempty"`      // 操作是否成功
	Message     string `json:"message,omitempty"`      // 操作结果消息
	DeviceCount int    `json:"device_count,omitempty"` // 剩余活跃设备数量
}

func (TOTPUnbind) Design() {
	Route("2fa/totp/unbind", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Payload[*TOTPUnbindReq]()
			Result[*TOTPUnbindRsp]()
		})
	})
}
