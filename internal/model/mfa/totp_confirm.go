package modelmfa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPConfirm completes TOTP device binding for the current user.
type TOTPConfirm struct {
	model.Empty
}

// TOTPConfirmReq confirms a pending TOTP binding challenge.
type TOTPConfirmReq struct {
	ChallengeID string `json:"challenge_id" validate:"required"`
	Code        string `json:"code" validate:"required,len=6"` // 6-digit TOTP code to confirm
	DeviceName  string `json:"device_name" validate:"required,max=100"`
}

// TOTPConfirmRsp returns the created device and one-time backup codes.
type TOTPConfirmRsp struct {
	DeviceID    string   `json:"device_id,omitempty"`
	Message     string   `json:"message,omitempty"`
	BackupCodes []string `json:"backup_codes,omitempty"` // One-time recovery codes shown only after binding
}

func (TOTPConfirm) Design() {
	Route("mfa/totp/confirm", func() {
		Create(func() {
			Service(true)
			Flatten()
			Filename("totp_confirm.go")
			Payload[*TOTPConfirmReq]()
			Result[*TOTPConfirmRsp]()
		})
	})
}
