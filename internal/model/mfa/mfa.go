package modelmfa

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type MFA struct {
	model.Empty
}

func (MFA) Design() {
	Route("mfa/totp/bind", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Result[*TOTPBindRsp]()
			Filename("totp_bind.go")
		})
	})

	Route("mfa/totp/check", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Public(true)
			Payload[*TOTPCheckReq]()
			Result[*TOTPCheckRsp]()
			Filename("totp_check.go")
		})
	})

	Route("mfa/totp/confirm", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Payload[*TOTPConfirmReq]()
			Result[*TOTPConfirmRsp]()
			Filename("totp_confirm.go")
		})
	})

	Route("mfa/totp/status", func() {
		List(func() {
			Enabled(true)
			Service(true)
			Result[*TOTPStatusRsp]()
			Filename("totp_status.go")
		})
	})

	Route("mfa/totp/unbind", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Payload[*TOTPUnbindReq]()
			Result[*TOTPUnbindRsp]()
			Filename("totp_unbind.go")
		})
	})

	Route("mfa/totp/verify", func() {
		Create(func() {
			Enabled(true)
			Service(true)
			Public(false) // Requires authentication
			Payload[*TOTPVerifyReq]()
			Result[*TOTPVerifyRsp]()
			Filename("totp_verify.go")
		})
	})
}
