package serviceemail

import (
	"strings"

	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// PasswordResetConfirmService handles the token confirmation step that finalizes
// the email-driven password reset flow.
type PasswordResetConfirmService struct {
	service.Base[*modelemail.PasswordResetConfirm, *modelemail.PasswordResetConfirmReq, *modelemail.PasswordResetConfirmRsp]
}

// Create completes the password reset flow by consuming the one-time token,
// delegating the password update to the configured user provider, and
// invalidating active sessions.
func (s *PasswordResetConfirmService) Create(ctx *types.ServiceContext, req *modelemail.PasswordResetConfirmReq) (rsp *modelemail.PasswordResetConfirmRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	flow, err := consumeEmailFlow(passwordResetContext(ctx), iamEmailFlowKindPasswordReset, req.Token)
	if err != nil {
		if errors.Is(err, errEmailFlowNotFound) || errors.Is(err, errEmailFlowExpired) {
			return &modelemail.PasswordResetConfirmRsp{
				Reset: false,
				Msg:   "invalid or expired password reset token",
			}, nil
		}
		log.Error("failed to consume password reset flow", err)
		return nil, errors.Wrap(err, "failed to consume password reset flow")
	}
	if strings.TrimSpace(flow.UserID) == "" {
		return nil, errors.New("password reset user id is required")
	}

	provider := currentUserProvider()
	user, err := provider.GetByID(ctx, flow.UserID)
	if err != nil {
		if errors.Is(err, ErrUserProviderNotConfigured) {
			log.Error("email user provider is not configured", err)
			return nil, newUserProviderNotConfiguredServiceError(err)
		}
		log.Error("failed to load password reset user", err)
		return nil, errors.Wrap(err, "failed to load password reset user")
	}
	if err = validUserSnapshot(user, flow.UserID); err != nil {
		log.Error("email user provider returned invalid password reset user", err)
		return nil, newUserProviderInvalidUserServiceError(err)
	}
	if normalizeUserEmail(user.Email) != normalizeEmailScope(flow.Email) {
		return &modelemail.PasswordResetConfirmRsp{
			Reset: false,
			Msg:   "invalid or expired password reset token",
		}, nil
	}

	if err = provider.ResetPassword(ctx, user.ID, req.NewPassword); err != nil {
		if errors.Is(err, ErrUserProviderNotConfigured) {
			log.Error("email user provider is not configured", err)
			return nil, newUserProviderNotConfiguredServiceError(err)
		}
		log.Error("failed to update password reset user", err)
		return nil, errors.Wrap(err, "failed to update password")
	}

	provider.InvalidateSessions(user.ID)
	return &modelemail.PasswordResetConfirmRsp{
		Reset: true,
		Msg:   "password reset successfully",
	}, nil
}
