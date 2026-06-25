package servicemfa

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/pquerna/otp/totp"
)

var (
	ErrLoginSecondFactorRequired = errors.New("login second factor required")
	ErrLoginSecondFactorConflict = errors.New("login second factor conflict")
	ErrLoginTOTPCodeInvalid      = errors.New("login TOTP code invalid")
	ErrLoginBackupCodeInvalid    = errors.New("login backup code invalid")
)

// LoginSecondFactor carries the second-factor fields accepted by IAM login.
type LoginSecondFactor struct {
	TOTPCode   string
	BackupCode string
}

// VerifyLoginSecondFactor enforces the MFA rules used during IAM login.
//
// The helper is intentionally login-specific: it skips all checks when the
// MFA module is disabled or the user has no active TOTP devices, requires
// exactly one submitted proof when MFA is active, updates LastUsedAt after a
// successful TOTP proof, and delegates recovery-code consumption to the shared
// transactional backup-code helper.
func VerifyLoginSecondFactor(ctx *types.ServiceContext, userID string, factor LoginSecondFactor) error {
	if !Enabled {
		return nil
	}

	userID = strings.TrimSpace(userID)
	if ctx == nil || userID == "" {
		return service.NewError(http.StatusUnauthorized, "authentication required")
	}

	devices, err := listActiveLoginTOTPDevices(ctx, userID)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return nil
	}

	totpCode := strings.TrimSpace(factor.TOTPCode)
	backupCode := strings.TrimSpace(factor.BackupCode)
	switch {
	case totpCode == "" && backupCode == "":
		return ErrLoginSecondFactorRequired
	case totpCode != "" && backupCode != "":
		return ErrLoginSecondFactorConflict
	case totpCode != "":
		return verifyLoginTOTPCode(ctx, devices, totpCode)
	default:
		return verifyLoginBackupCode(ctx, userID, backupCode)
	}
}

// listActiveLoginTOTPDevices loads the active devices that make login MFA mandatory.
func listActiveLoginTOTPDevices(ctx *types.ServiceContext, userID string) ([]*modelmfa.TOTPDevice, error) {
	devices := make([]*modelmfa.TOTPDevice, 0)
	if err := database.Database[*modelmfa.TOTPDevice](ctx).WithQuery(&modelmfa.TOTPDevice{
		UserID:   userID,
		IsActive: true,
	}).List(&devices); err != nil {
		return nil, errors.Wrap(err, "list login TOTP devices")
	}
	return devices, nil
}

// verifyLoginTOTPCode validates a login TOTP code and records the matched device usage.
func verifyLoginTOTPCode(ctx *types.ServiceContext, devices []*modelmfa.TOTPDevice, code string) error {
	device := findLoginTOTPDeviceByCode(devices, code)
	if device == nil {
		return ErrLoginTOTPCodeInvalid
	}

	now := time.Now()
	device.LastUsedAt = &now
	if err := database.Database[*modelmfa.TOTPDevice](ctx).Update(device); err != nil {
		return errors.Wrap(err, "update login TOTP device usage")
	}
	return nil
}

// verifyLoginBackupCode consumes one login recovery code and maps invalid input to login errors.
func verifyLoginBackupCode(ctx *types.ServiceContext, userID, code string) error {
	if err := ConsumeTOTPBackupCode(ctx, userID, code); err != nil {
		if errors.Is(err, errTOTPBackupCodeInvalid) {
			return ErrLoginBackupCodeInvalid
		}
		return errors.Wrap(err, "consume login TOTP backup code")
	}
	return nil
}

// findLoginTOTPDeviceByCode returns the first active device that accepts the code.
func findLoginTOTPDeviceByCode(devices []*modelmfa.TOTPDevice, code string) *modelmfa.TOTPDevice {
	for _, device := range devices {
		if device == nil || !device.IsActive {
			continue
		}
		if totp.Validate(code, device.Secret) {
			return device
		}
	}
	return nil
}
