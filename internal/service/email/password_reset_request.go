package serviceemail

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modelemail "github.com/hydroan/gst/internal/model/email"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// PasswordResetRequestService handles public password reset requests that start
// the email-driven password recovery flow.
type PasswordResetRequestService struct {
	service.Base[*modelemail.PasswordResetRequest, *modelemail.PasswordResetRequestReq, *modelemail.PasswordResetRequestRsp]
}

// passwordResetLookupUserByEmail resolves the account bound to the requested email
// and returns errEmailUserNotFound when no account uses it. The indirection keeps
// the production query simple and allows focused tests to stub the lookup without
// requiring a database fixture.
var passwordResetLookupUserByEmail = func(ctx *types.ServiceContext, email string) (*modeliamuser.User, error) {
	users := make([]*modeliamuser.User, 0, 1)
	queryEmail := email
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithLimit(1).
		WithQuery(&modeliamuser.User{Email: &queryEmail}).
		List(&users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, errEmailUserNotFound
	}
	return users[0], nil
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

	user, err := passwordResetLookupUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, errEmailUserNotFound) {
			return rsp, nil
		}
		log.Error("failed to load password reset user", err)
		return nil, errors.Wrap(err, "failed to load password reset user")
	}
	if !eligiblePasswordResetUser(user, email) {
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

// eligiblePasswordResetUser ensures the reset flow is only issued for an active
// account whose persisted email still matches the normalized request email.
func eligiblePasswordResetUser(user *modeliamuser.User, email string) bool {
	if user == nil || user.ID == "" {
		return false
	}
	if normalizePasswordResetEmail(user.Email) != email {
		return false
	}
	return user.Status == "" || user.Status == modeliamuser.UserStatusActive
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

// normalizePasswordResetEmail safely normalizes a nullable user email field.
func normalizePasswordResetEmail(email *string) string {
	if email == nil {
		return ""
	}
	return normalizeEmailScope(*email)
}

// passwordResetContext returns a non-nil context for flow operations triggered by
// the password reset services.
func passwordResetContext(ctx *types.ServiceContext) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx.Context()
}
