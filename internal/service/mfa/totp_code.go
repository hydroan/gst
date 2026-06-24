package servicemfa

import (
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/pquerna/otp/totp"
)

var errTOTPCodeInvalid = errors.New("invalid TOTP code")

// ValidateUserTOTPCode verifies a TOTP code against any active device owned by the user.
//
// This helper is for flows that need proof the current user still controls at
// least one active authenticator, such as fresh authentication before unbinding
// a different device. It never accepts a device ID from the caller.
func ValidateUserTOTPCode(ctx *types.ServiceContext, userID, code string) error {
	if ctx == nil || strings.TrimSpace(userID) == "" {
		return service.NewError(http.StatusUnauthorized, "authentication required")
	}

	devices := make([]*modelmfa.TOTPDevice, 0)
	if err := database.Database[*modelmfa.TOTPDevice](ctx.DatabaseContext()).WithQuery(&modelmfa.TOTPDevice{
		UserID:   strings.TrimSpace(userID),
		IsActive: true,
	}).List(&devices); err != nil {
		return errors.Wrap(err, "list TOTP devices")
	}

	return validateTOTPCodeForDevices(code, devices)
}

// validateTOTPCodeForDevices checks a code against an already-loaded active device list.
//
// Transactional callers use this to avoid issuing another query while holding a
// TOTPDevice lock.
func validateTOTPCodeForDevices(code string, devices []*modelmfa.TOTPDevice) error {
	if strings.TrimSpace(code) == "" {
		return errTOTPCodeInvalid
	}
	for _, device := range devices {
		if device == nil || !device.IsActive {
			continue
		}
		if totp.Validate(code, device.Secret) {
			return nil
		}
	}
	return errTOTPCodeInvalid
}
