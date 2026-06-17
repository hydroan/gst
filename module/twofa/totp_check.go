package twofa

import (
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	servicetwofa "github.com/hydroan/gst/internal/service/twofa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPCheck, *TOTPCheckReq, *TOTPCheckRsp] = (*TOTPCheckModule)(nil)

type (
	TOTPCheck       = modeltwofa.TOTPCheck
	TOTPCheckReq    = modeltwofa.TOTPCheckReq
	TOTPCheckRsp    = modeltwofa.TOTPCheckRsp
	TOTPCheckModule struct{}
)

func (*TOTPCheckModule) Service() types.Service[*TOTPCheck, *TOTPCheckReq, *TOTPCheckRsp] {
	return &servicetwofa.TOTPCheckService{}
}
func (*TOTPCheckModule) Route() string { return "2fa/totp/check" }
func (*TOTPCheckModule) Pub() bool     { return true }
func (*TOTPCheckModule) Param() string { return "id" }
