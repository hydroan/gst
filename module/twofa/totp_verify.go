package twofa

import (
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	servicetwofa "github.com/hydroan/gst/internal/service/twofa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPVerify, *TOTPVerifyReq, *TOTPVerifyRsp] = (*TOTPVerifyModule)(nil)

type (
	TOTPVerify       = modeltwofa.TOTPVerify
	TOTPVerifyReq    = modeltwofa.TOTPVerifyReq
	TOTPVerifyRsp    = modeltwofa.TOTPVerifyRsp
	TOTPVerifyModule struct{}
)

func (*TOTPVerifyModule) Service() types.Service[*TOTPVerify, *TOTPVerifyReq, *TOTPVerifyRsp] {
	return &servicetwofa.TOTPVerifyService{}
}
func (*TOTPVerifyModule) Route() string { return "2fa/totp/verify" }
func (*TOTPVerifyModule) Pub() bool     { return false }
func (*TOTPVerifyModule) Param() string { return "id" }
