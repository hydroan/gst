package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPStatus, *TOTPStatus, *TOTPStatusRsp] = (*TOTPStatusModule)(nil)

type (
	TOTPStatus       = modelmfa.TOTPStatus
	TOTPStatusRsp    = modelmfa.TOTPStatusRsp
	TOTPStatusModule struct{}
)

func (*TOTPStatusModule) Service() types.Service[*TOTPStatus, *TOTPStatus, *TOTPStatusRsp] {
	return &servicemfa.TOTPStatusService{}
}
func (*TOTPStatusModule) Route() string { return "mfa/totp/status" }
func (*TOTPStatusModule) Pub() bool     { return false }
func (*TOTPStatusModule) Param() string { return "id" }
