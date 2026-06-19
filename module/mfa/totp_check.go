package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPCheck, *TOTPCheckReq, *TOTPCheckRsp] = (*TOTPCheckModule)(nil)

type (
	TOTPCheck       = modelmfa.TOTPCheck
	TOTPCheckReq    = modelmfa.TOTPCheckReq
	TOTPCheckRsp    = modelmfa.TOTPCheckRsp
	TOTPCheckModule struct{}
)

func (*TOTPCheckModule) Service() types.Service[*TOTPCheck, *TOTPCheckReq, *TOTPCheckRsp] {
	return &servicemfa.TOTPCheckService{}
}
func (*TOTPCheckModule) Route() string { return "mfa/totp/check" }
func (*TOTPCheckModule) Pub() bool     { return true }
func (*TOTPCheckModule) Param() string { return "id" }
