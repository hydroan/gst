package servicemfa

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// TOTPUnbindService removes an active TOTP device after fresh authentication.
//
// The request must provide exactly one verification method: current password,
// TOTP code, or recovery code. Password verification is performed before the
// device transaction to avoid cross-model database work inside the device lock.
// TOTP and recovery-code verification run against the current user's active
// devices; recovery-code removal and target-device deletion share the same
// transaction so the code is consumed only when the unbind operation succeeds.
type TOTPUnbindService struct {
	service.Base[*modelmfa.TOTPUnbind, *modelmfa.TOTPUnbindReq, *modelmfa.TOTPUnbindRsp]
}

var errTOTPUnbindVerificationInvalid = errors.New("invalid verification")

// Create validates fresh authentication, removes the target device, and returns the remaining active count.
//
// The method rejects requests with zero or multiple verification methods,
// verifies the chosen proof, locks the current user's active devices, removes
// the target device, and reports how many active devices remain. Backup-code
// verification is performed in the same transaction as the device removal.
func (t *TOTPUnbindService) Create(ctx *types.ServiceContext, req *modelmfa.TOTPUnbindReq) (rsp *modelmfa.TOTPUnbindRsp, err error) {
	log := t.WithContext(ctx, ctx.GetPhase())

	if len(ctx.UserID()) == 0 {
		log.Errorz("user_id not found in context")
		return nil, service.NewError(http.StatusUnauthorized, "authentication required")
	}

	switch countTOTPUnbindVerificationMethods(req) {
	case 0:
		return newTOTPUnbindFailureRsp(ctx, "fresh authentication required")
	case 1:
	default:
		return newTOTPUnbindFailureRsp(ctx, "provide exactly one verification method")
	}

	if req.Password != "" {
		if !activeTOTPUnbindDeviceExists(ctx, ctx.UserID(), req.DeviceID) {
			log.Warnz("device not found or not active",
				zap.String("user_id", ctx.UserID()),
				zap.String("device_id", req.DeviceID))
			return newTOTPUnbindFailureRsp(ctx, "Device not found or already unbound")
		}
		if verifyErr := verifyTOTPUnbindPassword(ctx, ctx.UserID(), req.Password); verifyErr != nil {
			if isTOTPUnbindPasswordSystemError(verifyErr) {
				log.Errorz("failed to verify password for unbind",
					zap.String("user_id", ctx.UserID()),
					zap.String("device_id", req.DeviceID),
					zap.Error(verifyErr))
				return nil, verifyErr
			}
			log.Warnz("invalid password for unbind",
				zap.String("user_id", ctx.UserID()),
				zap.String("device_id", req.DeviceID),
				zap.Error(verifyErr))
			return newTOTPUnbindFailureRsp(ctx, "invalid verification")
		}
	}

	err = database.Database[*modelmfa.TOTPDevice](ctx).Transaction(func(tx types.Database[*modelmfa.TOTPDevice]) error {
		devices := make([]*modelmfa.TOTPDevice, 0)
		if listErr := tx.WithLock(consts.LockUpdate).WithQuery(&modelmfa.TOTPDevice{
			UserID:   ctx.UserID(),
			IsActive: true,
		}).List(&devices); listErr != nil {
			return errors.Wrap(listErr, "list active TOTP devices")
		}

		device := findTOTPUnbindDevice(devices, req.DeviceID)
		if device == nil {
			log.Warnz("device not found or not active",
				zap.String("user_id", ctx.UserID()),
				zap.String("device_id", req.DeviceID))
			rsp = &modelmfa.TOTPUnbindRsp{
				Success:     false,
				Message:     "Device not found or already unbound",
				DeviceCount: len(devices),
			}
			return nil
		}

		now := time.Now()
		if verifyErr := verifyTOTPUnbindFreshAuth(ctx, tx, req, devices, now); verifyErr != nil {
			if errors.Is(verifyErr, errTOTPUnbindVerificationInvalid) ||
				errors.Is(verifyErr, errTOTPCodeInvalid) ||
				errors.Is(verifyErr, errTOTPBackupCodeInvalid) {
				log.Warnz("invalid fresh authentication for unbind",
					zap.String("user_id", ctx.UserID()),
					zap.String("device_id", req.DeviceID),
					zap.Error(verifyErr))
				rsp = &modelmfa.TOTPUnbindRsp{
					Success:     false,
					Message:     "invalid verification",
					DeviceCount: len(devices),
				}
				return nil
			}
			return verifyErr
		}

		if deleteErr := tx.WithPurge(true).Delete(device); deleteErr != nil {
			return fmt.Errorf("failed to unbind device: %w", deleteErr)
		}

		rsp = &modelmfa.TOTPUnbindRsp{
			Success:     true,
			Message:     fmt.Sprintf("Device '%s' unbound successfully", device.DeviceName),
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
		return nil, errors.New("failed to build TOTP unbind response")
	}
	if rsp.Success {
		log.Infoz("totp device unbound successfully",
			zap.String("user_id", ctx.UserID()),
			zap.String("device_id", req.DeviceID),
			zap.Int("device_count", rsp.DeviceCount))
	}

	return rsp, nil
}

// newTOTPUnbindFailureRsp builds a failed response with the current active-device count.
func newTOTPUnbindFailureRsp(ctx *types.ServiceContext, message string) (*modelmfa.TOTPUnbindRsp, error) {
	count, err := countActiveTOTPUnbindDevices(ctx, ctx.UserID())
	if err != nil {
		return nil, err
	}
	return &modelmfa.TOTPUnbindRsp{
		Success:     false,
		Message:     message,
		DeviceCount: count,
	}, nil
}

// countActiveTOTPUnbindDevices returns the current user's active TOTP device count.
func countActiveTOTPUnbindDevices(ctx *types.ServiceContext, userID string) (int, error) {
	devices := make([]*modelmfa.TOTPDevice, 0)
	if err := database.Database[*modelmfa.TOTPDevice](ctx).WithQuery(&modelmfa.TOTPDevice{
		UserID:   userID,
		IsActive: true,
	}).List(&devices); err != nil {
		return 0, errors.Wrap(err, "count active TOTP devices")
	}
	return len(devices), nil
}

// countTOTPUnbindVerificationMethods counts which fresh-auth methods are present.
func countTOTPUnbindVerificationMethods(req *modelmfa.TOTPUnbindReq) int {
	count := 0
	if req.Password != "" {
		count++
	}
	if strings.TrimSpace(req.TOTPCode) != "" {
		count++
	}
	if strings.TrimSpace(req.BackupCode) != "" {
		count++
	}
	return count
}

// verifyTOTPUnbindFreshAuth validates the selected fresh-auth method in the device transaction.
//
// Password has already been validated before the transaction. TOTP verification
// accepts any active device owned by the current user. Recovery-code verification
// consumes the matching hash through the transaction-bound backup-code helper.
func verifyTOTPUnbindFreshAuth(
	ctx *types.ServiceContext,
	tx types.Database[*modelmfa.TOTPDevice],
	req *modelmfa.TOTPUnbindReq,
	devices []*modelmfa.TOTPDevice,
	now time.Time,
) error {
	switch {
	case req.Password != "":
		return nil
	case strings.TrimSpace(req.TOTPCode) != "":
		return validateTOTPCodeForDevices(req.TOTPCode, devices)
	case strings.TrimSpace(req.BackupCode) != "":
		return consumeTOTPBackupCodeInTx(tx, ctx.UserID(), req.BackupCode, now)
	default:
		return errTOTPUnbindVerificationInvalid
	}
}

// verifyTOTPUnbindPassword validates the current account's password for fresh auth.
func verifyTOTPUnbindPassword(ctx *types.ServiceContext, userID, password string) error {
	account, err := currentAccountAuthenticator().AuthenticateByAccountID(ctx, userID, password)
	if err != nil {
		if errors.Is(err, ErrAccountAuthenticatorNotConfigured) {
			return newAccountAuthenticatorNotConfiguredServiceError(err)
		}
		if errors.Is(err, ErrAccountAuthenticationFailed) {
			return errTOTPUnbindVerificationInvalid
		}
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify password", err)
	}
	if err := validateAuthenticatedAccount(account, userID); err != nil {
		return newAccountAuthenticatorInvalidAccountServiceError(err)
	}
	return nil
}

func isTOTPUnbindPasswordSystemError(err error) bool {
	var serviceErr *service.Error
	return errors.As(err, &serviceErr)
}

// activeTOTPUnbindDeviceExists checks target ownership before password validation.
//
// The password path performs this preflight outside the device transaction to
// keep IAM user lookup out of the TOTPDevice transaction while preserving the
// same device-not-found response semantics.
func activeTOTPUnbindDeviceExists(ctx *types.ServiceContext, userID, deviceID string) bool {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false
	}

	device := new(modelmfa.TOTPDevice)
	query := &modelmfa.TOTPDevice{
		UserID:   userID,
		IsActive: true,
	}
	query.Base.ID = deviceID
	return database.Database[*modelmfa.TOTPDevice](ctx).WithQuery(query).First(device) == nil
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
