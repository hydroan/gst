package servicemfa

import (
	"net/http"
	"strings"
	"time"

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
type TOTPVerifyService struct {
	service.Base[*modelmfa.TOTPVerify, *modelmfa.TOTPVerifyReq, *modelmfa.TOTPVerifyRsp]
}

// Create verifies a 6-digit TOTP code.
//
// The method first enforces authentication and validates the submitted code
// shape. Codes that are well-formed but do not match any selected active device
// return a negative verification response instead of a service error.
func (t *TOTPVerifyService) Create(ctx *types.ServiceContext, req *modelmfa.TOTPVerifyReq) (rsp *modelmfa.TOTPVerifyRsp, err error) {
	log := t.WithContext(ctx, ctx.Phase())

	if len(ctx.UserID()) == 0 {
		log.Errorz("user_id not found in context")
		return nil, service.NewError(http.StatusUnauthorized, "authentication required")
	}

	code := strings.TrimSpace(req.TOTPCode)
	if code == "" {
		log.Errorz("totp code is empty")
		return nil, service.NewError(http.StatusBadRequest, "TOTP code is required")
	}
	if !isSixDigitTOTPCode(code) {
		log.Warnz("invalid totp code format", zap.String("user_id", ctx.UserID()))
		return nil, service.NewError(http.StatusBadRequest, "TOTP code must be 6 digits")
	}

	devices := make([]*modelmfa.TOTPDevice, 0)
	query := &modelmfa.TOTPDevice{
		UserID:   ctx.UserID(),
		IsActive: true,
	}

	if strings.TrimSpace(req.DeviceID) != "" {
		query.ID = strings.TrimSpace(req.DeviceID)
	}

	if err = database.Database[*modelmfa.TOTPDevice](ctx).WithQuery(query).List(&devices); err != nil {
		log.Errorz("failed to list totp devices", zap.Error(err))
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to retrieve device information", err)
	}

	if len(devices) == 0 {
		log.Warnz("no active totp devices found", zap.String("user_id", ctx.UserID()))
		return nil, service.NewError(http.StatusBadRequest, "no active TOTP devices found")
	}

	var validDevice *modelmfa.TOTPDevice

	for _, device := range devices {
		if totp.Validate(code, device.Secret) {
			validDevice = device
			break
		}
	}

	if validDevice == nil {
		log.Warnz("invalid verification code",
			zap.String("user_id", ctx.UserID()))
		return &modelmfa.TOTPVerifyRsp{
			Valid:   false,
			Message: "invalid verification code",
		}, nil
	}

	now := time.Now().UTC()
	validDevice.LastUsedAt = &now

	// A failed usage-timestamp update never fails the verification itself.
	if err = database.Database[*modelmfa.TOTPDevice](ctx).Update(validDevice); err != nil {
		log.Errorz("failed to update device", zap.Error(err))
	}

	log.Infoz("totp verification successful",
		zap.String("user_id", ctx.UserID()),
		zap.String("device_id", validDevice.ID))

	return &modelmfa.TOTPVerifyRsp{
		Valid:   true,
		Message: "verification successful",
	}, nil
}

// isSixDigitTOTPCode reports whether code is exactly six ASCII digits.
func isSixDigitTOTPCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
