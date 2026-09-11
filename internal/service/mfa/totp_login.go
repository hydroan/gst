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

// LoginSecondFactorVerifier enforces the MFA rules used during IAM login. It
// is the gate installed through authn.SetLoginSecondFactorVerifier: module/mfa
// installs it on the add path, and project-owned assembly code installs it on
// the copy path. Nothing here installs itself, so the gate is never armed by a
// package import alone.
//
// Accounts without active TOTP devices pass untouched. Enrolled accounts must
// submit exactly one proof: a TOTP code, consumed against replay on success,
// or a recovery code, removed transactionally. Each proof first spends one
// attempt from the user's login budget, and a verified proof resets it. Per
// the authn contract the verifier owns the client-facing error shape, and
// clients branch on status plus message: the stable 401
// authn.MsgSecondFactorRequired tells a login UI to prompt for the code, 401
// with other messages reports an invalid proof, 400 reports both proofs
// arriving at once, and 429 reports a spent budget, which refuses even a
// correct proof until its window ends.
func LoginSecondFactorVerifier(ctx *types.ServiceContext, userID string, factor authn.LoginSecondFactor) error {
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
		// authn.MsgSecondFactorRequired is a stable client contract: login UIs match
		// it to prompt for the code, replacing the pre-login check endpoint.
		return service.NewError(http.StatusUnauthorized, authn.MsgSecondFactorRequired)
	case totpCode != "" && backupCode != "":
		return service.NewError(http.StatusBadRequest, "provide exactly one second factor")
	}

	// Only a submitted proof spends the budget; the rejections above guess
	// nothing. The attempt is paid before the proof is checked, so a counter
	// that cannot be reached refuses the login without touching a recovery code.
	if err = reserveTOTPVerificationAttempt(ctx, totpVerificationLogin, userID); err != nil {
		if errors.Is(err, errTOTPVerificationLocked) {
			return service.NewError(http.StatusTooManyRequests, "too many failed verification attempts")
		}
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify second factor", err)
	}
	if totpCode != "" {
		return verifyLoginTOTPCode(ctx, devices, totpCode)
	}
	return verifyLoginBackupCode(ctx, userID, backupCode)
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
// records the matched device usage, and resets the login attempt budget.
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
	// Narrowed to the usage column: this lock-free write must not resurrect
	// recovery-code hashes that a concurrent consumption already removed.
	if err := database.Database[*modelmfa.TOTPDevice](ctx).
		WithSelect(colTOTPDeviceLastUsedAt).
		Update(device); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify second factor", err)
	}
	clearTOTPVerificationFailures(ctx, device.UserID, totpVerificationLogin)
	return nil
}

// verifyLoginBackupCode consumes one login recovery code, maps invalid input
// to the login error contract, and resets the login attempt budget.
func verifyLoginBackupCode(ctx *types.ServiceContext, userID, code string) error {
	if err := consumeTOTPBackupCode(ctx, userID, code); err != nil {
		if errors.Is(err, errTOTPBackupCodeInvalid) {
			return service.NewError(http.StatusUnauthorized, "invalid backup code")
		}
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify second factor", err)
	}
	clearTOTPVerificationFailures(ctx, userID, totpVerificationLogin)
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
