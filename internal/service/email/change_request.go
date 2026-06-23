package serviceemail

import (
	"strings"

	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ChangeRequestService handles authenticated requests that start the email
// change flow for the current account.
type ChangeRequestService struct {
	service.Base[*modelemail.ChangeRequest, *modelemail.ChangeRequestReq, *modelemail.ChangeRequestRsp]
}

// Create validates the current password, checks the target email, and issues
// one-time confirmation and cancellation tokens for the email change flow.
func (s *ChangeRequestService) Create(ctx *types.ServiceContext, req *modelemail.ChangeRequestReq) (rsp *modelemail.ChangeRequestRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())
	user, newEmail, rsp, err := prepareEmailChangeRequest(ctx, req.NewEmail)
	if err != nil || user == nil {
		if err != nil {
			log.Error("failed to prepare email change request", err)
		}
		return rsp, err
	}

	if err = verifyEmailChangePassword(ctx, user.ID, req.CurrentPassword); err != nil {
		log.Error("failed to verify email change password", err)
		return nil, err
	}
	if err = startEmailChangeFlow(ctx, user, newEmail, true); err != nil {
		log.Error("failed to start email change flow", err)
		return nil, err
	}

	return rsp, nil
}

// prepareEmailChangeRequest loads the current user and validates whether the new
// email can enter the change flow.
func prepareEmailChangeRequest(ctx *types.ServiceContext, newEmail string) (*UserSnapshot, string, *modelemail.ChangeRequestRsp, error) {
	if ctx == nil || strings.TrimSpace(ctx.UserID) == "" {
		return nil, "", nil, errors.New("authentication required")
	}

	user, err := currentUserProvider().GetByID(ctx, ctx.UserID)
	if err != nil {
		if errors.Is(err, ErrUserProviderNotConfigured) {
			return nil, "", nil, newUserProviderNotConfiguredServiceError(err)
		}
		return nil, "", nil, errors.Wrap(err, "failed to load current user")
	}
	if err = validUserSnapshot(user, ctx.UserID); err != nil {
		return nil, "", nil, newUserProviderInvalidUserServiceError(err)
	}

	normalizedNewEmail := normalizeEmailScope(newEmail)
	if err = validateEmailChangeTarget(ctx, user, normalizedNewEmail); err != nil {
		return nil, "", nil, err
	}

	return user, normalizedNewEmail, &modelemail.ChangeRequestRsp{
		Msg: "email change request submitted successfully",
	}, nil
}

// validateEmailChangeTarget ensures the current account can start an email
// change flow to the requested target address.
func validateEmailChangeTarget(ctx *types.ServiceContext, user *UserSnapshot, newEmail string) error {
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return errors.New("current user is required")
	}
	if !user.Active {
		return errors.New("current user is not active")
	}
	currentEmail := normalizeUserEmail(user.Email)
	if currentEmail == "" {
		return errors.New("current email is required")
	}
	if newEmail == "" {
		return errors.New("new email is required")
	}
	if newEmail == currentEmail {
		return errors.New("new email must be different from current email")
	}

	existingUser, err := currentUserProvider().FindByEmail(ctx, newEmail)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil
		}
		if errors.Is(err, ErrUserProviderNotConfigured) {
			return newUserProviderNotConfiguredServiceError(err)
		}
		return errors.Wrap(err, "failed to lookup target email")
	}
	if existingUser != nil && existingUser.ID != user.ID {
		return errors.New("new email is already in use")
	}

	return nil
}

// verifyEmailChangePassword re-authenticates the current user before issuing
// email change tokens.
func verifyEmailChangePassword(ctx *types.ServiceContext, userID, password string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("current user id is required")
	}
	if err := currentUserProvider().VerifyPassword(ctx, userID, password); err != nil {
		if errors.Is(err, ErrUserAuthenticationFailed) {
			return errors.New("current password is incorrect")
		}
		if errors.Is(err, ErrUserProviderNotConfigured) {
			return newUserProviderNotConfiguredServiceError(err)
		}
		return errors.Wrap(err, "failed to verify current password")
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("current password is incorrect")
	}
	return nil
}

// startEmailChangeFlow issues the required tokens and dispatches the email
// change notifications for the target flow.
func startEmailChangeFlow(ctx *types.ServiceContext, user *UserSnapshot, newEmail string, includeCancel bool) error {
	currentEmail := normalizeUserEmail(user.Email)
	if err := clearEmailChangeCancellation(ctx.Context(), user.ID, currentEmail, newEmail); err != nil {
		return errors.Wrap(err, "failed to clear previous email change cancellation")
	}
	if _, err := reserveEmailThrottle(ctx.Context(), iamEmailFlowKindChangeConfirm, emailThrottleRequest, newEmail, 0); err != nil {
		if errors.Is(err, errEmailFlowThrottled) {
			return errors.Wrap(err, "email change confirmation throttled")
		}
		return errors.Wrap(err, "failed to reserve email change confirmation throttle")
	}
	if includeCancel {
		if _, err := reserveEmailThrottle(ctx.Context(), iamEmailFlowKindChangeCancel, emailThrottleRequest, currentEmail, 0); err != nil {
			if errors.Is(err, errEmailFlowThrottled) {
				return errors.Wrap(err, "email change cancellation throttled")
			}
			return errors.Wrap(err, "failed to reserve email change cancellation throttle")
		}
	}

	confirmToken, confirmFlow, err := issueEmailFlow(ctx.Context(), iamEmailFlowKindChangeConfirm, iamEmailFlowState{
		UserID:   user.ID,
		OldEmail: currentEmail,
		NewEmail: newEmail,
		Email:    newEmail,
	}, 0)
	if err != nil {
		return errors.Wrap(err, "failed to issue email change confirmation flow")
	}
	if err = dispatchEmail(ctx.Context(), changeConfirmDelivery(confirmToken, confirmFlow)); err != nil {
		return errors.Wrap(err, "failed to dispatch email change confirmation")
	}

	if !includeCancel {
		return nil
	}

	cancelToken, cancelFlow, err := issueEmailFlow(ctx.Context(), iamEmailFlowKindChangeCancel, iamEmailFlowState{
		UserID:   user.ID,
		OldEmail: currentEmail,
		NewEmail: newEmail,
		Email:    currentEmail,
	}, 0)
	if err != nil {
		return errors.Wrap(err, "failed to issue email change cancellation flow")
	}
	if err = dispatchEmail(ctx.Context(), changeCancelDelivery(cancelToken, cancelFlow)); err != nil {
		return errors.Wrap(err, "failed to dispatch email change cancellation")
	}

	return nil
}

// changeConfirmDelivery builds the email payload delivered to the new email address.
func changeConfirmDelivery(token string, flow iamEmailFlowState) emailDelivery {
	return emailDelivery{
		To:       flow.NewEmail,
		Subject:  "Email change confirmation",
		Template: "iam/email/change-confirm",
		Data: map[string]any{
			"token":      token,
			"user_id":    flow.UserID,
			"new_email":  flow.NewEmail,
			"old_email":  flow.OldEmail,
			"expires_at": flow.ExpiresAt,
		},
	}
}

// changeCancelDelivery builds the email payload delivered to the current email
// address so the user can cancel an unexpected change request.
func changeCancelDelivery(token string, flow iamEmailFlowState) emailDelivery {
	return emailDelivery{
		To:       flow.OldEmail,
		Subject:  "Email change cancellation",
		Template: "iam/email/change-cancel",
		Data: map[string]any{
			"token":      token,
			"user_id":    flow.UserID,
			"new_email":  flow.NewEmail,
			"old_email":  flow.OldEmail,
			"expires_at": flow.ExpiresAt,
		},
	}
}
