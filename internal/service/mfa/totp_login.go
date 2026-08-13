package servicemfa

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authn"
	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/pquerna/otp/totp"
)

// The MFA service arms login second-factor enforcement at package
// initialization: importing this package — through module/mfa on the add path
// or the copied service package on the copy path — is what installs the
// verifier. No separate switch exists.
func init() {
	authn.SetLoginSecondFactorVerifier(verifyLoginSecondFactor)
}

// verifyLoginSecondFactor enforces the MFA rules used during IAM login.
//
// Accounts without active TOTP devices pass untouched. Enrolled accounts must
// submit exactly one proof: a TOTP code, consumed against replay on success,
// or a recovery code, removed transactionally. Per the authn contract the
// verifier owns the client-facing error shape, and clients branch on status
// plus message: the stable 401 message "second factor required" tells a login
// UI to prompt for the code, 401 with other messages reports an invalid
// proof, and 400 reports both proofs arriving at once.
func verifyLoginSecondFactor(ctx *types.ServiceContext, userID string, factor authn.LoginSecondFactor) error {
	userID = strings.TrimSpace(userID)
	if ctx == nil || userID == "" {
		return service.NewError(http.StatusUnauthorized, "authentication required")
	}

	devices, err := listActiveLoginTOTPDevices(ctx, userID)
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify second factor", err)
	}
	if len(devices) == 0 {
		return nil
	}

	totpCode := strings.TrimSpace(factor.TOTPCode)
	backupCode := strings.TrimSpace(factor.BackupCode)
	switch {
	case totpCode == "" && backupCode == "":
		// "second factor required" is a stable client contract: login UIs match
		// it to prompt for the code, replacing the pre-login check endpoint.
		return service.NewError(http.StatusUnauthorized, "second factor required")
	case totpCode != "" && backupCode != "":
		return service.NewError(http.StatusBadRequest, "provide exactly one second factor")
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

// verifyLoginTOTPCode validates a login TOTP code, consumes it against replay,
// and records the matched device usage.
func verifyLoginTOTPCode(ctx *types.ServiceContext, devices []*modelmfa.TOTPDevice, code string) error {
	device := findLoginTOTPDeviceByCode(devices, code)
	if device == nil {
		return service.NewError(http.StatusUnauthorized, "invalid TOTP code")
	}

	// A replayed code fails login exactly like a wrong one.
	if err := markTOTPCodeUsed(ctx, device.UserID, code); err != nil {
		if errors.Is(err, errTOTPCodeReplayed) {
			return service.NewError(http.StatusUnauthorized, "invalid TOTP code")
		}
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify second factor", err)
	}

	now := time.Now().UTC()
	device.LastUsedAt = &now
	// Narrowed for the same reason as verify: this lock-free write must not
	// resurrect concurrently consumed recovery-code hashes.
	if err := database.Database[*modelmfa.TOTPDevice](ctx).
		WithSelect(colTOTPDeviceLastUsedAt).
		Update(device); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify second factor", err)
	}
	return nil
}

// verifyLoginBackupCode consumes one login recovery code and maps invalid input
// to the login error contract.
func verifyLoginBackupCode(ctx *types.ServiceContext, userID, code string) error {
	if err := ConsumeTOTPBackupCode(ctx, userID, code); err != nil {
		if errors.Is(err, errTOTPBackupCodeInvalid) {
			return service.NewError(http.StatusUnauthorized, "invalid backup code")
		}
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify second factor", err)
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
