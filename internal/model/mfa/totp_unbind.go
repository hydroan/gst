package modelmfa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPUnbind removes a TOTP device from the current user.
type TOTPUnbind struct {
	model.Empty
}

// TOTPUnbindReq requires one fresh verification method — a TOTP code or a
// recovery code — before removing a device. The first factor alone can never
// switch off the second one, so a password is deliberately not accepted; an
// account that lost both its device and its recovery codes goes through
// administrative reset instead.
type TOTPUnbindReq struct {
	DeviceID   string `json:"device_id" validate:"required"`
	TOTPCode   string `json:"totp_code,omitempty" validate:"omitempty,len=6,numeric"`
	BackupCode string `json:"backup_code,omitempty"`
}

// TOTPUnbindRsp returns the remaining active-device count after removal. The
// HTTP status already reports success, and the removed device is the one the
// caller named, so no other fields are needed.
type TOTPUnbindRsp struct {
	DeviceCount int `json:"device_count"`
}

func (TOTPUnbind) Design() {
	Route("mfa/totp/unbind", func() {
		Create(func() {
			Service()
			Flatten()
			Filename("totp_unbind.go")
			Payload[*TOTPUnbindReq]()
			Result[*TOTPUnbindRsp]()
		})
	})
}
