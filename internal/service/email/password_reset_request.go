package serviceemail

import (
	"context"

	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// PasswordResetRequestService handles public password reset requests that start
// the email-driven password recovery flow.
type PasswordResetRequestService struct {
	service.Base[*modelemail.PasswordResetRequest, *modelemail.PasswordResetRequestReq, *modelemail.PasswordResetRequestRsp]
}

// Create starts the password reset flow for the provided email address.
// It always returns the same public-facing message for accepted requests so the
// caller cannot infer whether the account exists, while still enforcing throttle
// limits before any token is created or email is sent.
func (s *PasswordResetRequestService) Create(ctx *types.ServiceContext, req *modelemail.PasswordResetRequestReq) (rsp *modelemail.PasswordResetRequestRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())
	rsp = &modelemail.PasswordResetRequestRsp{Msg: publicAcceptedMessage(iamEmailFlowKindPasswordReset)}

	email := normalizeEmailScope(req.Email)
	if email == "" {
		return rsp, nil
	}

	if _, err = reserveEmailThrottle(ctx.Context(), iamEmailFlowKindPasswordReset, emailThrottleRequest, email, 0); err != nil {
		if errors.Is(err, errEmailFlowThrottled) {
			return rsp, nil
		}
		log.Error("failed to reserve password reset throttle", err)
		return nil, errors.Wrap(err, "failed to reserve password reset throttle")
	}

	user, err := currentAccountGateway().FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return rsp, nil
		}
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			log.Error("email account gateway is not configured", err)
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		log.Error("failed to load password reset account", err)
		return nil, errors.Wrap(err, "failed to load password reset account")
	}
	if !eligiblePasswordResetAccount(user, email) {
		return rsp, nil
	}

	token, flow, err := issueEmailFlow(ctx.Context(), iamEmailFlowKindPasswordReset, iamEmailFlowState{
		UserID: user.ID,
		Email:  email,
	}, 0)
	if err != nil {
		log.Error("failed to issue password reset flow", err)
		return nil, errors.Wrap(err, "failed to issue password reset flow")
	}

	if err = dispatchEmail(ctx.Context(), passwordResetDelivery(token, flow)); err != nil {
		log.Error("failed to dispatch password reset email", err)
		return nil, errors.Wrap(err, "failed to dispatch password reset email")
	}

	return rsp, nil
}

// eligiblePasswordResetAccount ensures the reset flow is only issued for an active
// account whose persisted email still matches the normalized request email.
func eligiblePasswordResetAccount(user *AccountSnapshot, email string) bool {
	if user == nil || user.ID == "" {
		return false
	}
	if normalizeAccountEmail(user.Email) != email {
		return false
	}
	return user.Active
}

// passwordResetDelivery builds the delivery payload consumed by the configured
// email sender. The template data mirrors the flow state so downstream renderers
// can build reset links or customized message bodies.
func passwordResetDelivery(token string, flow iamEmailFlowState) emailDelivery {
	return emailDelivery{
		To:       flow.Email,
		Subject:  "Password reset",
		Template: "iam/email/password-reset",
		Data: map[string]any{
			"token":      token,
			"user_id":    flow.UserID,
			"email":      flow.Email,
			"expires_at": flow.ExpiresAt,
		},
	}
}

// normalizeAccountEmail normalizes an email address loaded from the host user store.
func normalizeAccountEmail(email string) string {
	return normalizeEmailScope(email)
}

// passwordResetContext returns a non-nil context for flow operations triggered by
// the password reset services.
func passwordResetContext(ctx *types.ServiceContext) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx.Context()
}
