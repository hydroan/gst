package modelmfa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TOTPCheck represents a pre-login MFA requirement check.
type TOTPCheck struct {
	model.Empty
}

type TOTPCheckReq struct {
	Username string `json:"username" validate:"required"` // Username
	Password string `json:"password" validate:"required"`
}

type TOTPCheckRsp struct {
	RequiresMFA bool   `json:"requires_mfa"`      // Whether MFA verification is required
	Message     string `json:"message,omitempty"` // Response message
}

func (TOTPCheck) Design() {
	Route("mfa/totp/check", func() {
		Create(func() {
			Service()
			Flatten()
			Public()
			Filename("totp_check.go")
			Payload[*TOTPCheckReq]()
			Result[*TOTPCheckRsp]()
		})
	})
}
