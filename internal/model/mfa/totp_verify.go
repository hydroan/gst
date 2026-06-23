package modelmfa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPVerify represents a request to verify a TOTP code.
type TOTPVerify struct {
	model.Empty
}

// TOTPVerifyReq verifies a 6-digit TOTP code for the current user.
type TOTPVerifyReq struct {
	TOTPCode string `json:"totp_code" validate:"required,len=6,numeric"` // 6-digit TOTP code
	DeviceID string `json:"device_id,omitempty"`                         // Optional: specific device ID
}

// TOTPVerifyRsp returns the TOTP verification result.
type TOTPVerifyRsp struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

func (TOTPVerify) Design() {
	Route("mfa/totp/verify", func() {
		Create(func() {
			Service(true)
			Flatten()
			Filename("totp_verify.go")
			Payload[*TOTPVerifyReq]()
			Result[*TOTPVerifyRsp]()
		})
	})
}
