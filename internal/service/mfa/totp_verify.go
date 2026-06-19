package servicemfa

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modelmfa "github.com/hydroan/gst/internal/model/mfa"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
)

// TOTPVerifyService verifies a logged-in user's MFA code.
//
// Standard TOTP verification checks the submitted code against the user's
// active devices and updates the matching device's last-used timestamp.
// Backup-code verification delegates to the recovery-code service, which
// validates and consumes the matching hash transactionally.
type TOTPVerifyService struct {
	service.Base[*modelmfa.TOTPVerify, *modelmfa.TOTPVerifyReq, *modelmfa.TOTPVerifyRsp]
}

// Create verifies either a TOTP code or a one-time recovery code.
//
// The method first enforces authentication and non-empty input. Recovery codes
// are consumed through the shared backup-code helper; normal TOTP codes are
// checked against the selected device or all active devices for the user.
func (t *TOTPVerifyService) Create(ctx *types.ServiceContext, req *modelmfa.TOTPVerifyReq) (rsp *modelmfa.TOTPVerifyRsp, err error) {
	log := t.WithServiceContext(ctx, ctx.GetPhase())

	if len(ctx.UserID) == 0 {
		log.Errorz("user_id not found in context")
		return &modelmfa.TOTPVerifyRsp{
			Valid:   false,
			Message: "authentication required",
		}, types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}

	if len(req.Code) == 0 {
		log.Errorz("code is empty")
		return &modelmfa.TOTPVerifyRsp{
			Valid:   false,
			Message: "verification code is required",
		}, errors.New("verification code is required")
	}

	if req.IsBackup {
		if err = ConsumeTOTPBackupCode(ctx, ctx.UserID, req.Code); err != nil {
			log.Warnz("invalid backup code", zap.String("user_id", ctx.UserID), zap.Error(err))
			return &modelmfa.TOTPVerifyRsp{
				Valid:   false,
				Message: "invalid verification code",
			}, nil
		}
		log.Infoz("backup code verification successful", zap.String("user_id", ctx.UserID))
		return &modelmfa.TOTPVerifyRsp{
			Valid:   true,
			Message: "verification successful",
		}, nil
	}

	devices := make([]*modelmfa.TOTPDevice, 0)
	query := &modelmfa.TOTPDevice{
		UserID:   ctx.UserID,
		IsActive: true,
	}

	if len(req.DeviceID) > 0 {
		query.Base.ID = req.DeviceID
	}

	if err = database.Database[*modelmfa.TOTPDevice](ctx.DatabaseContext()).WithQuery(query).List(&devices); err != nil {
		log.Errorz("failed to list totp devices", zap.Error(err))
		return &modelmfa.TOTPVerifyRsp{
			Valid:   false,
			Message: "failed to retrieve device information",
		}, fmt.Errorf("failed to list devices: %w", err)
	}

	if len(devices) == 0 {
		log.Warnz("no active totp devices found", zap.String("user_id", ctx.UserID))
		return &modelmfa.TOTPVerifyRsp{
			Valid:   false,
			Message: "no active TOTP devices found",
		}, errors.New("no active TOTP devices found")
	}

	var validDevice *modelmfa.TOTPDevice

	for _, device := range devices {
		if totp.Validate(req.Code, device.Secret) {
			validDevice = device
			break
		}
	}

	if validDevice == nil {
		log.Warnz("invalid verification code",
			zap.String("user_id", ctx.UserID),
			zap.Bool("is_backup", req.IsBackup))
		return &modelmfa.TOTPVerifyRsp{
			Valid:   false,
			Message: "invalid verification code",
		}, nil
	}

	now := time.Now()
	validDevice.LastUsedAt = &now

	if err = database.Database[*modelmfa.TOTPDevice](ctx.DatabaseContext()).Update(validDevice); err != nil {
		log.Errorz("failed to update device", zap.Error(err))
		log.Warnz("device update failed but verification succeeded")
	}

	log.Infoz("totp verification successful",
		zap.String("user_id", ctx.UserID),
		zap.String("device_id", validDevice.ID),
		zap.Bool("is_backup", req.IsBackup))

	return &modelmfa.TOTPVerifyRsp{
		Valid:   true,
		Message: "verification successful",
	}, nil
}
