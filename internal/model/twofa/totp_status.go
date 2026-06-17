package modeltwofa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPStatus 获取用户的 2FA 状态
type TOTPStatus struct {
	model.Empty
}
type TOTPStatusRsp struct {
	Enabled     bool             `json:"enabled,omitempty"`      // Whether 2FA is enabled
	DeviceCount int              `json:"device_count,omitempty"` // Number of active devices
	Devices     []TOTPDeviceInfo `json:"devices,omitempty"`      // List of devices (without secrets)
}

type TOTPDeviceInfo struct {
	ID         string  `json:"id"`
	DeviceName string  `json:"device_name"`
	IsActive   bool    `json:"is_active"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

func (TOTPStatus) Design() {
	Route("2fa/totp/status", func() {
		List(func() {
			Enabled(true)
			Service(true)
			Result[*TOTPStatusRsp]()
		})
	})
}
