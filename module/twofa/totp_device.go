package twofa

import (
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	servicetwofa "github.com/hydroan/gst/internal/service/twofa"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*TOTPDevice, *TOTPDevice, *TOTPDevice] = (*TOTPDeviceModule)(nil)

type (
	TOTPDevice       = modeltwofa.TOTPDevice
	TOTPDeviceModule struct{}
)

func (*TOTPDeviceModule) Service() types.Service[*TOTPDevice, *TOTPDevice, *TOTPDevice] {
	return &servicetwofa.TOTPDeviceService{}
}
func (*TOTPDeviceModule) Route() string { return "2fa/totp/devices" }
func (*TOTPDeviceModule) Pub() bool     { return false }
func (*TOTPDeviceModule) Param() string { return "id" }
