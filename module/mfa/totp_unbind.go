package mfa

import (
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	servicemfa "github.com/hydroan/gst/internal/service/mfa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPUnbind, *TOTPUnbindReq, *TOTPUnbindRsp] = (*TOTPUnbindModule)(nil)

type (
	TOTPUnbind       = modelmfa.TOTPUnbind
	TOTPUnbindReq    = modelmfa.TOTPUnbindReq
	TOTPUnbindRsp    = modelmfa.TOTPUnbindRsp
	TOTPUnbindModule struct{}
)

func (*TOTPUnbindModule) Service() types.Service[*TOTPUnbind, *TOTPUnbindReq, *TOTPUnbindRsp] {
	return &servicemfa.TOTPUnbindService{}
}
func (*TOTPUnbindModule) Route() string { return "mfa/totp/unbind" }
func (*TOTPUnbindModule) Pub() bool     { return false }
func (*TOTPUnbindModule) Param() string { return "id" }
