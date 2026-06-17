package twofa

import (
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	servicetwofa "github.com/hydroan/gst/internal/service/twofa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPUnbind, *TOTPUnbindReq, *TOTPUnbindRsp] = (*TOTPUnbindModule)(nil)

type (
	TOTPUnbind       = modeltwofa.TOTPUnbind
	TOTPUnbindReq    = modeltwofa.TOTPUnbindReq
	TOTPUnbindRsp    = modeltwofa.TOTPUnbindRsp
	TOTPUnbindModule struct{}
)

func (*TOTPUnbindModule) Service() types.Service[*TOTPUnbind, *TOTPUnbindReq, *TOTPUnbindRsp] {
	return &servicetwofa.TOTPUnbindService{}
}
func (*TOTPUnbindModule) Route() string { return "2fa/totp/unbind" }
func (*TOTPUnbindModule) Pub() bool     { return false }
func (*TOTPUnbindModule) Param() string { return "id" }
