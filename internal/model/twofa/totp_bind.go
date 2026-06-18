package modeltwofa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPBind starts TOTP device binding for the current user.
type TOTPBind struct {
	model.Empty
}

// TOTPBindRsp returns the pending binding challenge and authenticator setup data.
type TOTPBindRsp struct {
	ChallengeID string `json:"challenge_id,omitempty"`
	OtpauthURL  string `json:"otpauth_url,omitempty"`   // TOTP provisioning URL
	QRCodeImage string `json:"qr_code_image,omitempty"` // Base64-encoded QR code image data
	Issuer      string `json:"issuer,omitempty"`        // Application issuer name
	AccountName string `json:"account_name,omitempty"`  // User account name
}

func (TOTPBind) Design() {
	Route("2fa/totp/bind", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Result[*TOTPBindRsp]()
		})
	})
}
