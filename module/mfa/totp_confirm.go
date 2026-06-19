package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPConfirm, *TOTPConfirmReq, *TOTPConfirmRsp] = (*TOTPConfirmModule)(nil)

type (
	TOTPConfirm       = modelmfa.TOTPConfirm
	TOTPConfirmReq    = modelmfa.TOTPConfirmReq
	TOTPConfirmRsp    = modelmfa.TOTPConfirmRsp
	TOTPConfirmModule struct{}
)

func (*TOTPConfirmModule) Service() types.Service[*TOTPConfirm, *TOTPConfirmReq, *TOTPConfirmRsp] {
	return &servicemfa.TOTPConfirmService{}
}
func (*TOTPConfirmModule) Route() string { return "mfa/totp/confirm" }
func (*TOTPConfirmModule) Pub() bool     { return false }
func (*TOTPConfirmModule) Param() string { return "id" }
