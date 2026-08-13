package servicemfa

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
)

// TOTPUnbindService removes an active TOTP device after fresh authentication.
//
// The request must provide exactly one verification method: a TOTP code or a
// recovery code, both proving possession of the second factor itself — the
// password alone deliberately cannot switch MFA off. Verification runs against
// the current user's active devices; recovery-code removal and target-device
// deletion share the same transaction so the code is consumed only when the
// unbind operation succeeds.
//
// Failures answer through service errors like the rest of the module: 400 for
// malformed requests, 401 for failed fresh authentication, 404 for a missing
// target device. Credentials are always judged before the target device is
// looked up, so an unauthenticated caller cannot probe device existence.
type TOTPUnbindService struct {
	service.Base[*modelmfa.TOTPUnbind, *modelmfa.TOTPUnbindReq, *modelmfa.TOTPUnbindRsp]
}

// Create validates fresh authentication, removes the target device, and returns the remaining active count.
func (t *TOTPUnbindService) Create(ctx *types.ServiceContext, req *modelmfa.TOTPUnbindReq) (rsp *modelmfa.TOTPUnbindRsp, err error) {
	log := t.WithContext(ctx, ctx.Phase())

	if len(ctx.UserID()) == 0 {
		return nil, service.NewError(http.StatusUnauthorized, "authentication required")
	}
	if strings.TrimSpace(req.DeviceID) == "" {
		return nil, service.NewError(http.StatusBadRequest, "device_id is required")
	}
	switch countTOTPUnbindVerificationMethods(req) {
	case 0:
		return nil, service.NewError(http.StatusBadRequest, "fresh authentication required")
	case 1:
	default:
		return nil, service.NewError(http.StatusBadRequest, "provide exactly one verification method")
	}

	userID := ctx.UserID()
	err = database.Transaction(ctx, func(ctx context.Context) error {
		devices := make([]*modelmfa.TOTPDevice, 0)
		if listErr := database.Database[*modelmfa.TOTPDevice](ctx).WithLock(consts.LockUpdate).WithQuery(&modelmfa.TOTPDevice{
			UserID:   userID,
			IsActive: true,
		}).List(&devices); listErr != nil {
			return service.NewErrorWithCause(http.StatusInternalServerError, "failed to list active TOTP devices", listErr)
		}

		// Fresh authentication is judged before the target device so a failed
		// proof never reveals whether the device exists. A service error here
		// rolls the transaction back, which also restores a consumed recovery
		// code; a burned replay marker for a valid TOTP code is the accepted
		// cost of erring toward safety.
		now := time.Now().UTC()
		if verifyErr := verifyTOTPUnbindFreshAuth(ctx, userID, req, devices, now); verifyErr != nil {
			if errors.Is(verifyErr, errTOTPCodeInvalid) ||
				errors.Is(verifyErr, errTOTPCodeReplayed) ||
				errors.Is(verifyErr, errTOTPBackupCodeInvalid) {
				log.Warnz("invalid fresh authentication for unbind",
					zap.String("user_id", userID),
					zap.String("device_id", req.DeviceID),
					zap.Error(verifyErr))
				return service.NewError(http.StatusUnauthorized, "invalid verification")
			}
			return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify fresh authentication", verifyErr)
		}

		device := findTOTPUnbindDevice(devices, req.DeviceID)
		if device == nil {
			log.Warnz("device not found or not active",
				zap.String("user_id", userID),
				zap.String("device_id", req.DeviceID))
			return service.NewError(http.StatusNotFound, "device not found or already unbound")
		}

		if deleteErr := database.Database[*modelmfa.TOTPDevice](ctx).WithPurge(true).Delete(device); deleteErr != nil {
			return service.NewErrorWithCause(http.StatusInternalServerError, "failed to unbind device", deleteErr)
		}

		rsp = &modelmfa.TOTPUnbindRsp{
			DeviceCount: countRemainingTOTPDevices(devices, device.ID),
		}
		return nil
	})
	if err != nil {
		log.Errorz("failed to unbind device",
			zap.String("user_id", ctx.UserID()),
			zap.String("device_id", req.DeviceID),
			zap.Error(err))
		return nil, err
	}

	if rsp == nil {
		return nil, service.NewError(http.StatusInternalServerError, "failed to build TOTP unbind response")
	}
	log.Infoz("totp device unbound successfully",
		zap.String("user_id", ctx.UserID()),
		zap.String("device_id", req.DeviceID),
		zap.Int("device_count", rsp.DeviceCount))

	return rsp, nil
}

// countTOTPUnbindVerificationMethods counts which fresh-auth methods are present.
func countTOTPUnbindVerificationMethods(req *modelmfa.TOTPUnbindReq) int {
	count := 0
	if strings.TrimSpace(req.TOTPCode) != "" {
		count++
	}
	if strings.TrimSpace(req.BackupCode) != "" {
		count++
	}
	return count
}

// errTOTPUnbindVerificationInvalid guards the defensive default below. Create
// enforces exactly one verification method before the transaction, so hitting
// it is a programming error and surfaces as a 500 through the generic branch.
var errTOTPUnbindVerificationInvalid = errors.New("invalid verification")

// verifyTOTPUnbindFreshAuth validates the selected fresh-auth method inside the
// device transaction.
//
// TOTP verification accepts any active device owned by the current user. ctx
// carries the transaction created by the caller, so recovery-code verification
// consumes the matching hash in the same transaction as the device removal.
func verifyTOTPUnbindFreshAuth(
	ctx context.Context,
	userID string,
	req *modelmfa.TOTPUnbindReq,
	devices []*modelmfa.TOTPDevice,
	now time.Time,
) error {
	switch {
	case strings.TrimSpace(req.TOTPCode) != "":
		return validateTOTPCodeForDevices(ctx, userID, req.TOTPCode, devices)
	case strings.TrimSpace(req.BackupCode) != "":
		return consumeTOTPBackupCodeInTx(ctx, userID, req.BackupCode, now)
	default:
		return errTOTPUnbindVerificationInvalid
	}
}

var errTOTPCodeInvalid = errors.New("invalid TOTP code")

// validateTOTPCodeForDevices checks a code against an already-loaded active
// device list and consumes it on success.
//
// Transactional callers use this to avoid issuing another query while holding a
// TOTPDevice lock. The replay marker is written even when the surrounding
// transaction later rolls back; erring toward a burned code is the safe side.
func validateTOTPCodeForDevices(ctx context.Context, userID, code string, devices []*modelmfa.TOTPDevice) error {
	if strings.TrimSpace(code) == "" {
		return errTOTPCodeInvalid
	}
	for _, device := range devices {
		if device == nil || !device.IsActive {
			continue
		}
		if totp.Validate(code, device.Secret) {
			return markTOTPCodeUsed(ctx, userID, code)
		}
	}
	return errTOTPCodeInvalid
}

// findTOTPUnbindDevice selects the target active device from the locked device list.
func findTOTPUnbindDevice(devices []*modelmfa.TOTPDevice, deviceID string) *modelmfa.TOTPDevice {
	deviceID = strings.TrimSpace(deviceID)
	for _, device := range devices {
		if device == nil {
			continue
		}
		if device.ID == deviceID {
			return device
		}
	}
	return nil
}

// countRemainingTOTPDevices returns the active-device count after removing one device.
func countRemainingTOTPDevices(devices []*modelmfa.TOTPDevice, removedDeviceID string) int {
	count := 0
	for _, device := range devices {
		if device == nil || device.ID == removedDeviceID {
			continue
		}
		count++
	}
	return count
}
