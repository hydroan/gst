package serviceemail

import (
	"net/http"
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
	log := s.WithContext(ctx, ctx.Phase())
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

// prepareEmailChangeRequest loads the current account and validates whether the new
// email can enter the change flow.
func prepareEmailChangeRequest(ctx *types.ServiceContext, newEmail string) (*AccountSnapshot, string, *modelemail.ChangeRequestRsp, error) {
	if ctx == nil || strings.TrimSpace(ctx.UserID()) == "" {
		return nil, "", nil, service.NewError(http.StatusUnauthorized, "authentication required")
	}

	user, err := currentAccountGateway().GetByID(ctx, ctx.UserID())
	if err != nil {
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			return nil, "", nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		return nil, "", nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load current account", err)
	}
	if err = validAccountSnapshot(user, ctx.UserID()); err != nil {
		return nil, "", nil, newAccountGatewayInvalidAccountServiceError(err)
	}

	normalizedNewEmail := normalizeEmailScope(newEmail)
	if err = validateEmailChangeTarget(ctx, user, normalizedNewEmail); err != nil {
		return nil, "", nil, err
	}

	return user, normalizedNewEmail, &modelemail.ChangeRequestRsp{
		Msg: "email change request submitted successfully",
	}, nil
}

// verifyEmailChangePassword re-authenticates the current account before issuing
// email change tokens.
func verifyEmailChangePassword(ctx *types.ServiceContext, userID, password string) error {
	if strings.TrimSpace(userID) == "" {
		return service.NewError(http.StatusBadRequest, "current account id is required")
	}
	if err := currentAccountGateway().VerifyPassword(ctx, userID, password); err != nil {
		if errors.Is(err, ErrAccountAuthenticationFailed) {
			return service.NewError(http.StatusBadRequest, "current password is incorrect")
		}
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			return newAccountGatewayNotConfiguredServiceError(err)
		}
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to verify current password", err)
	}
	if strings.TrimSpace(password) == "" {
		return service.NewError(http.StatusBadRequest, "current password is incorrect")
	}
	return nil
}

// startEmailChangeFlow issues the required tokens and dispatches the email
// change notifications for the target flow.
func startEmailChangeFlow(ctx *types.ServiceContext, user *AccountSnapshot, newEmail string, includeCancel bool) error {
	currentEmail := normalizeAccountEmail(user.Email)
	if err := clearEmailChangeCancellation(ctx, user.ID, currentEmail, newEmail); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to clear previous email change cancellation", err)
	}
	if _, err := reserveEmailThrottle(ctx, iamEmailFlowKindChangeConfirm, emailThrottleRequest, newEmail, 0); err != nil {
		if errors.Is(err, errEmailFlowThrottled) {
			return service.NewErrorWithCause(http.StatusInternalServerError, "email change confirmation throttled", err)
		}
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to reserve email change confirmation throttle", err)
	}
	if includeCancel {
		if _, err := reserveEmailThrottle(ctx, iamEmailFlowKindChangeCancel, emailThrottleRequest, currentEmail, 0); err != nil {
			if errors.Is(err, errEmailFlowThrottled) {
				return service.NewErrorWithCause(http.StatusInternalServerError, "email change cancellation throttled", err)
			}
			return service.NewErrorWithCause(http.StatusInternalServerError, "failed to reserve email change cancellation throttle", err)
		}
	}

	confirmToken, confirmFlow, err := issueEmailFlow(ctx, iamEmailFlowKindChangeConfirm, iamEmailFlowState{
		UserID:   user.ID,
		OldEmail: currentEmail,
		NewEmail: newEmail,
		Email:    newEmail,
	})
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to issue email change confirmation flow", err)
	}
	if err = dispatchEmail(ctx, changeConfirmDelivery(confirmToken, confirmFlow)); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to dispatch email change confirmation", err)
	}

	if !includeCancel {
		return nil
	}

	cancelToken, cancelFlow, err := issueEmailFlow(ctx, iamEmailFlowKindChangeCancel, iamEmailFlowState{
		UserID:   user.ID,
		OldEmail: currentEmail,
		NewEmail: newEmail,
		Email:    currentEmail,
	})
	if err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to issue email change cancellation flow", err)
	}
	if err = dispatchEmail(ctx, changeCancelDelivery(cancelToken, cancelFlow)); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to dispatch email change cancellation", err)
	}

	return nil
}
