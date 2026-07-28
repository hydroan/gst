package servicemfa

import (
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
	log := t.WithContext(ctx, ctx.Phase())

	if len(ctx.UserID()) == 0 {
		return nil, service.NewError(http.StatusUnauthorized, "authentication required")
	}
	sessionID, err := currentTOTPBindSessionID(ctx)
	if err != nil {
		return nil, err
	}

	challenge, err := loadTOTPBindChallenge(ctx, req.ChallengeID)
	if err != nil {
		if errors.Is(err, errTOTPBindChallengeNotFound) ||
			errors.Is(err, errTOTPBindChallengeExpired) ||
			errors.Is(err, errTOTPBindChallengeInvalid) {
			return nil, service.NewErrorWithCause(http.StatusBadRequest, "invalid or expired TOTP binding challenge", err)
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load TOTP binding challenge", err)
	}
	if challenge.UserID != ctx.UserID() || challenge.SessionID != sessionID {
		return nil, service.NewError(http.StatusBadRequest, "invalid or expired TOTP binding challenge")
	}

	valid := totp.Validate(req.Code, challenge.Secret)
	if !valid {
		return nil, service.NewError(http.StatusBadRequest, "invalid TOTP code")
	}

	log.Infoz("totp code validated successfully", zap.String("user_id", ctx.UserID()))

	devices := make([]*modelmfa.TOTPDevice, 0)
	if err = database.Database[*modelmfa.TOTPDevice](ctx).WithQuery(&modelmfa.TOTPDevice{
		UserID: ctx.UserID(),
		Secret: challenge.Secret,
	}).WithLimit(1).List(&devices); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list devices", err)
	}
	if len(devices) > 0 {
		return nil, service.NewError(http.StatusConflict, "device already bound")
	}

	backupCodes, err := GenerateTOTPBackupCodes()
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to generate backup codes", err)
	}
	backupCodeHashes, err := HashTOTPBackupCodes(backupCodes)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to hash backup codes", err)
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
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to save device", err)
	}

	log.Infoz("totp device created successfully",
		zap.String("user_id", ctx.UserID()),
		zap.String("device_id", device.ID))

	if err = consumeTOTPBindChallenge(ctx, req.ChallengeID); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to consume TOTP binding challenge", err)
	}

	rsp = &modelmfa.TOTPConfirmRsp{
		DeviceID:    device.ID,
		Message:     "TOTP device confirmed and activated successfully",
		BackupCodes: backupCodes,
	}

	return rsp, nil
}
