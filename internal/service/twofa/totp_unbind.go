package servicetwofa

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type TOTPUnbindService struct {
	service.Base[*modeltwofa.TOTPUnbind, *modeltwofa.TOTPUnbindReq, *modeltwofa.TOTPUnbindRsp]
}

var errTOTPUnbindVerificationInvalid = errors.New("invalid verification")

func (t *TOTPUnbindService) Create(ctx *types.ServiceContext, req *modeltwofa.TOTPUnbindReq) (rsp *modeltwofa.TOTPUnbindRsp, err error) {
	log := t.WithServiceContext(ctx, ctx.GetPhase())

	if len(ctx.UserID) == 0 {
		log.Errorz("user_id not found in context")
		return nil, types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}

	switch countTOTPUnbindVerificationMethods(req) {
	case 0:
		return &modeltwofa.TOTPUnbindRsp{
			Success: false,
			Message: "fresh authentication required",
		}, nil
	case 1:
	default:
		return &modeltwofa.TOTPUnbindRsp{
			Success: false,
			Message: "provide exactly one verification method",
		}, nil
	}

	if req.Password != "" {
		if !activeTOTPUnbindDeviceExists(ctx, ctx.UserID, req.DeviceID) {
			log.Warnz("device not found or not active",
				zap.String("user_id", ctx.UserID),
				zap.String("device_id", req.DeviceID))
			return &modeltwofa.TOTPUnbindRsp{
				Success: false,
				Message: "Device not found or already unbound",
			}, nil
		}
		if verifyErr := verifyTOTPUnbindPassword(ctx, ctx.UserID, req.Password); verifyErr != nil {
			log.Warnz("invalid password for unbind",
				zap.String("user_id", ctx.UserID),
				zap.String("device_id", req.DeviceID),
				zap.Error(verifyErr))
			return &modeltwofa.TOTPUnbindRsp{
				Success: false,
				Message: "invalid verification",
			}, nil
		}
	}

	err = database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext()).Transaction(func(tx types.Database[*modeltwofa.TOTPDevice]) error {
		devices := make([]*modeltwofa.TOTPDevice, 0)
		if listErr := tx.WithLock(consts.LockUpdate).WithQuery(&modeltwofa.TOTPDevice{
			UserID:   ctx.UserID,
			IsActive: true,
		}).List(&devices); listErr != nil {
			return errors.Wrap(listErr, "list active TOTP devices")
		}

		device := findTOTPUnbindDevice(devices, req.DeviceID)
		if device == nil {
			log.Warnz("device not found or not active",
				zap.String("user_id", ctx.UserID),
				zap.String("device_id", req.DeviceID))
			rsp = &modeltwofa.TOTPUnbindRsp{
				Success: false,
				Message: "Device not found or already unbound",
			}
			return nil
		}

		now := time.Now()
		if verifyErr := verifyTOTPUnbindFreshAuth(ctx, tx, req, devices, now); verifyErr != nil {
			if errors.Is(verifyErr, errTOTPUnbindVerificationInvalid) ||
				errors.Is(verifyErr, errTOTPCodeInvalid) ||
				errors.Is(verifyErr, errTOTPBackupCodeInvalid) {
				log.Warnz("invalid fresh authentication for unbind",
					zap.String("user_id", ctx.UserID),
					zap.String("device_id", req.DeviceID),
					zap.Error(verifyErr))
				rsp = &modeltwofa.TOTPUnbindRsp{
					Success: false,
					Message: "invalid verification",
				}
				return nil
			}
			return verifyErr
		}

		if deleteErr := tx.WithPurge(true).Delete(device); deleteErr != nil {
			return fmt.Errorf("failed to unbind device: %w", deleteErr)
		}

		rsp = &modeltwofa.TOTPUnbindRsp{
			Success:     true,
			Message:     fmt.Sprintf("Device '%s' unbound successfully", device.DeviceName),
			DeviceCount: countRemainingTOTPDevices(devices, device.ID),
		}
		return nil
	})
	if err != nil {
		log.Errorz("failed to unbind device",
			zap.String("user_id", ctx.UserID),
			zap.String("device_id", req.DeviceID),
			zap.Error(err))
		return nil, err
	}

	if rsp == nil {
		return nil, errors.New("failed to build TOTP unbind response")
	}
	if rsp.Success {
		log.Infoz("totp device unbound successfully",
			zap.String("user_id", ctx.UserID),
			zap.String("device_id", req.DeviceID),
			zap.Int("device_count", rsp.DeviceCount))
	}

	return rsp, nil
}

func countTOTPUnbindVerificationMethods(req *modeltwofa.TOTPUnbindReq) int {
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

func verifyTOTPUnbindFreshAuth(
	ctx *types.ServiceContext,
	tx types.Database[*modeltwofa.TOTPDevice],
	req *modeltwofa.TOTPUnbindReq,
	devices []*modeltwofa.TOTPDevice,
	now time.Time,
) error {
	switch {
	case req.Password != "":
		return nil
	case strings.TrimSpace(req.TOTPCode) != "":
		return validateTOTPCodeForDevices(req.TOTPCode, devices)
	case strings.TrimSpace(req.BackupCode) != "":
		return consumeTOTPBackupCodeInTx(tx, ctx.UserID, req.BackupCode, now)
	default:
		return errTOTPUnbindVerificationInvalid
	}
}

func verifyTOTPUnbindPassword(ctx *types.ServiceContext, userID, password string) error {
	user := new(modeliamuser.User)
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).Get(user, userID); err != nil {
		return errTOTPUnbindVerificationInvalid
	}
	if user.Status != modeliamuser.UserStatusActive {
		return errTOTPUnbindVerificationInvalid
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return errTOTPUnbindVerificationInvalid
	}
	return nil
}

func activeTOTPUnbindDeviceExists(ctx *types.ServiceContext, userID, deviceID string) bool {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false
	}

	device := new(modeltwofa.TOTPDevice)
	query := &modeltwofa.TOTPDevice{
		UserID:   userID,
		IsActive: true,
	}
	query.Base.ID = deviceID
	return database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext()).WithQuery(query).First(device) == nil
}

func findTOTPUnbindDevice(devices []*modeltwofa.TOTPDevice, deviceID string) *modeltwofa.TOTPDevice {
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

func countRemainingTOTPDevices(devices []*modeltwofa.TOTPDevice, removedDeviceID string) int {
	count := 0
	for _, device := range devices {
		if device == nil || device.ID == removedDeviceID {
			continue
		}
		count++
	}
	return count
}
