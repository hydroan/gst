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
// server-held secret, consumes the challenge, and then creates the active
// device. It returns one-time recovery codes only in this response while
// storing bcrypt hashes in the device record. Consuming the challenge before
// any storage work means a failure can never leave an activated device whose
// recovery codes were generated but never delivered.
type TOTPConfirmService struct {
	service.Base[*modelmfa.TOTPConfirm, *modelmfa.TOTPConfirmReq, *modelmfa.TOTPConfirmRsp]
}

// Create turns a valid binding challenge into an active TOTP device.
//
// The method verifies challenge ownership, checks and consumes the submitted
// TOTP code, consumes the challenge, creates recovery codes, stores only their
// hashes, and persists the device. The (user_id, secret) unique index is the
// authoritative duplicate guard; the List pre-check only provides the friendly
// conflict message on the common path.
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
	// A wrong code above keeps the challenge alive for retry; a replayed code
	// answers like a wrong one and also keeps the challenge, so the user can
	// retry with the next period's code.
	if err = markTOTPCodeUsed(ctx, ctx.UserID(), req.Code); err != nil {
		if errors.Is(err, errTOTPCodeReplayed) {
			return nil, service.NewError(http.StatusBadRequest, "invalid TOTP code")
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to confirm TOTP binding", err)
	}

	log.Infoz("totp code validated successfully", zap.String("user_id", ctx.UserID()))

	// Consuming the challenge before any storage work is the atomicity anchor
	// of this flow: once consumed it cannot start a second device creation, and
	// a later storage failure aborts before recovery codes exist, so nothing is
	// silently lost. Concurrent confirms that already loaded the challenge are
	// stopped by the (user_id, secret) unique index instead.
	if err = consumeTOTPBindChallenge(ctx, req.ChallengeID); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to consume TOTP binding challenge", err)
	}

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

	now := time.Now().UTC()
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

	rsp = &modelmfa.TOTPConfirmRsp{
		DeviceID:    device.ID,
		Message:     "TOTP device confirmed and activated successfully",
		BackupCodes: backupCodes,
	}

	return rsp, nil
}
