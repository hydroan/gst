package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPVerify, *TOTPVerifyReq, *TOTPVerifyRsp] = (*TOTPVerifyModule)(nil)

type (
	TOTPVerify       = modelmfa.TOTPVerify
	TOTPVerifyReq    = modelmfa.TOTPVerifyReq
	TOTPVerifyRsp    = modelmfa.TOTPVerifyRsp
	TOTPVerifyModule struct{}
)

func (*TOTPVerifyModule) Service() types.Service[*TOTPVerify, *TOTPVerifyReq, *TOTPVerifyRsp] {
	return &servicemfa.TOTPVerifyService{}
}
func (*TOTPVerifyModule) Route() string { return "mfa/totp/verify" }
func (*TOTPVerifyModule) Pub() bool     { return false }
func (*TOTPVerifyModule) Param() string { return "id" }
