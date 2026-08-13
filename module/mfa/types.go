package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
)

// TOTP API aliases.
type (
	TOTPBindRsp = modelmfa.TOTPBindRsp

	TOTPCheckReq = modelmfa.TOTPCheckReq
	TOTPCheckRsp = modelmfa.TOTPCheckRsp

	TOTPConfirmReq = modelmfa.TOTPConfirmReq
	TOTPConfirmRsp = modelmfa.TOTPConfirmRsp

	TOTPStatusRsp = modelmfa.TOTPStatusRsp

	TOTPUnbindReq = modelmfa.TOTPUnbindReq
	TOTPUnbindRsp = modelmfa.TOTPUnbindRsp

	TOTPVerifyReq = modelmfa.TOTPVerifyReq
	TOTPVerifyRsp = modelmfa.TOTPVerifyRsp
)
