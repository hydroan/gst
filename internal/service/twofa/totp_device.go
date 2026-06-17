package servicetwofa

import (
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	"github.com/hydroan/gst/service"
)

type TOTPDeviceService struct {
	service.Base[*modeltwofa.TOTPDevice, *modeltwofa.TOTPDevice, *modeltwofa.TOTPDevice]
}
