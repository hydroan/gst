package twofa

import (
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	servicetwofa "github.com/hydroan/gst/internal/service/twofa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPStatus, *TOTPStatus, *TOTPStatusRsp] = (*TOTPStatusModule)(nil)

type (
	TOTPStatus       = modeltwofa.TOTPStatus
	TOTPStatusRsp    = modeltwofa.TOTPStatusRsp
	TOTPStatusModule struct{}
)

func (*TOTPStatusModule) Service() types.Service[*TOTPStatus, *TOTPStatus, *TOTPStatusRsp] {
	return &servicetwofa.TOTPStatusService{}
}
func (*TOTPStatusModule) Route() string { return "2fa/totp/status" }
func (*TOTPStatusModule) Pub() bool     { return false }
func (*TOTPStatusModule) Param() string { return "id" }
