package twofa

import (
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	servicetwofa "github.com/hydroan/gst/internal/service/twofa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPBind, *TOTPBind, *TOTPBindRsp] = (*TOTPBindModule)(nil)

type (
	TOTPBind       = modeltwofa.TOTPBind
	TOTPBindRsp    = modeltwofa.TOTPBindRsp
	TOTPBindModule struct{}
)

func (*TOTPBindModule) Service() types.Service[*TOTPBind, *TOTPBind, *TOTPBindRsp] {
	return &servicetwofa.TOTPBindService{}
}
func (*TOTPBindModule) Route() string { return "2fa/totp/bind" }
func (*TOTPBindModule) Pub() bool     { return false }
func (*TOTPBindModule) Param() string { return "id" }
