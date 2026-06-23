package modelmfa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPStatus represents a request for the current user's MFA status.
type TOTPStatus struct {
	model.Empty
}
type TOTPStatusRsp struct {
	Enabled     bool             `json:"enabled"`      // Whether MFA is enabled
	DeviceCount int              `json:"device_count"` // Number of active devices
	Devices     []TOTPDeviceInfo `json:"devices"`      // Active devices without secrets
}

type TOTPDeviceInfo struct {
	ID         string  `json:"id"`
	DeviceName string  `json:"device_name"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

func (TOTPStatus) Design() {
	Route("mfa/totp/status", func() {
		List(func() {
			Service(true)
			Flatten()
			Filename("totp_status.go")
			Result[*TOTPStatusRsp]()
		})
	})
}
