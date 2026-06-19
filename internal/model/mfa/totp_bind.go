package modelmfa

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
	ChallengeID        string `json:"challenge_id,omitempty"`
	OtpauthURL         string `json:"otpauth_url,omitempty"`            // TOTP provisioning URL
	QRCodeImageDataURL string `json:"qr_code_image_data_url,omitempty"` // PNG QR code as a data URL
	Issuer             string `json:"issuer,omitempty"`                 // Application issuer name
	AccountName        string `json:"account_name,omitempty"`           // User account name
}

func (TOTPBind) Design() {
	Route("mfa/totp/bind", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Result[*TOTPBindRsp]()
		})
	})
}
