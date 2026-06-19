package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPBind, *TOTPBind, *TOTPBindRsp] = (*TOTPBindModule)(nil)

type (
	TOTPBind       = modelmfa.TOTPBind
	TOTPBindRsp    = modelmfa.TOTPBindRsp
	TOTPBindModule struct{}
)

func (*TOTPBindModule) Service() types.Service[*TOTPBind, *TOTPBind, *TOTPBindRsp] {
	return &servicemfa.TOTPBindService{}
}
func (*TOTPBindModule) Route() string { return "mfa/totp/bind" }
func (*TOTPBindModule) Pub() bool     { return false }
func (*TOTPBindModule) Param() string { return "id" }
