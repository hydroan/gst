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

// TOTPConfirmService completes a pending TOTP binding flow.
//
// The service loads the cached binding challenge, ensures it belongs to the
// current user and session, validates the submitted TOTP code against the
// server-held secret, and then creates the active device. It returns one-time
// recovery codes only in this response while storing bcrypt hashes in the device
// record. The binding challenge is consumed only after the device is saved.
type TOTPConfirmService struct {
	service.Base[*modelmfa.TOTPConfirm, *modelmfa.TOTPConfirmReq, *modelmfa.TOTPConfirmRsp]
}

// Create turns a valid binding challenge into an active TOTP device.
//
// The method verifies challenge ownership, checks the submitted TOTP code,
// prevents duplicate binding for the same secret, creates recovery codes, stores
// only their hashes, persists the device, and then consumes the challenge.
func (t *TOTPConfirmService) Create(ctx *types.ServiceContext, req *modelmfa.TOTPConfirmReq) (rsp *modelmfa.TOTPConfirmRsp, err error) {
	log := t.WithContext(ctx, ctx.GetPhase())

	if len(ctx.UserID()) == 0 {
		log.Errorz("user_id not found in context")
		return nil, service.NewError(http.StatusUnauthorized, "authentication required")
	}
	sessionID, err := currentTOTPBindSessionID(ctx)
	if err != nil {
		log.Errorz("session_id not found in context")
		return nil, err
	}

	challenge, err := loadTOTPBindChallenge(ctx, req.ChallengeID)
	if err != nil {
		if errors.Is(err, errTOTPBindChallengeNotFound) ||
			errors.Is(err, errTOTPBindChallengeExpired) ||
			errors.Is(err, errTOTPBindChallengeInvalid) {
			log.Warnz("invalid or expired totp bind challenge", zap.String("user_id", ctx.UserID()))
			return nil, errors.New("invalid or expired TOTP binding challenge")
		}
		log.Errorz("failed to load TOTP bind challenge", zap.Error(err))
		return nil, errors.Wrap(err, "failed to load TOTP binding challenge")
	}
	if challenge.UserID != ctx.UserID() || challenge.SessionID != sessionID {
		log.Warnz("totp bind challenge does not match current session", zap.String("user_id", ctx.UserID()))
		return nil, errors.New("invalid or expired TOTP binding challenge")
	}

	valid := totp.Validate(req.Code, challenge.Secret)
	if !valid {
		log.Warnz("invalid totp code", zap.String("user_id", ctx.UserID()))
		return nil, errors.New("invalid TOTP code")
	}

	log.Infoz("totp code validated successfully", zap.String("user_id", ctx.UserID()))

	devices := make([]*modelmfa.TOTPDevice, 0)
	if err = database.Database[*modelmfa.TOTPDevice](ctx).WithQuery(&modelmfa.TOTPDevice{
		UserID: ctx.UserID(),
		Secret: challenge.Secret,
	}).WithLimit(1).List(&devices); err != nil {
		log.Errorz("failed to list devices", zap.Error(err))
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	if len(devices) > 0 {
		log.Warnz("device already exists", zap.String("user_id", ctx.UserID()), zap.String("device_id", devices[0].ID))
		return nil, errors.New("device already bound")
	}

	backupCodes, err := GenerateTOTPBackupCodes()
	if err != nil {
		log.Errorz("failed to generate backup codes", zap.Error(err))
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}
	backupCodeHashes, err := HashTOTPBackupCodes(backupCodes)
	if err != nil {
		log.Errorz("failed to hash backup codes", zap.Error(err))
		return nil, fmt.Errorf("failed to hash backup codes: %w", err)
	}

	now := time.Now()
	device := &modelmfa.TOTPDevice{
		UserID:           ctx.UserID(),
		DeviceName:       req.DeviceName,
		Secret:           challenge.Secret,
		BackupCodeHashes: backupCodeHashes,
		IsActive:         true,
		LastUsedAt:       &now,
	}

	if err = database.Database[*modelmfa.TOTPDevice](ctx).Create(device); err != nil {
		log.Errorz("failed to create totp device", zap.Error(err))
		return nil, fmt.Errorf("failed to save device: %w", err)
	}

	log.Infoz("totp device created successfully",
		zap.String("user_id", ctx.UserID()),
		zap.String("device_id", device.ID))

	if err = consumeTOTPBindChallenge(ctx, req.ChallengeID); err != nil {
		log.Errorz("failed to consume TOTP bind challenge", zap.Error(err))
		return nil, errors.Wrap(err, "failed to consume TOTP binding challenge")
	}

	rsp = &modelmfa.TOTPConfirmRsp{
		DeviceID:    device.ID,
		Message:     "TOTP device confirmed and activated successfully",
		BackupCodes: backupCodes,
	}

	return rsp, nil
}
