package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
)

// TOTP API aliases.
type (
	TOTPBindRsp = modelmfa.TOTPBindRsp

	TOTPConfirmReq = modelmfa.TOTPConfirmReq
	TOTPConfirmRsp = modelmfa.TOTPConfirmRsp

	TOTPStatusRsp = modelmfa.TOTPStatusRsp

	TOTPUnbindReq = modelmfa.TOTPUnbindReq
	TOTPUnbindRsp = modelmfa.TOTPUnbindRsp

	AdminTOTPResetRsp = modelmfa.AdminTOTPResetRsp
)
