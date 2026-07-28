package serviceemail

import (
	"net/http"
	"strings"

	"github.com/hydroan/gst/service"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
)

// validateEmailChangeTarget ensures the current account can start an email
// change flow to the requested target address.
func validateEmailChangeTarget(ctx *types.ServiceContext, user *AccountSnapshot, newEmail string) error {
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return service.NewError(http.StatusBadRequest, "current account is required")
	}
	if !user.Active {
		return service.NewError(http.StatusBadRequest, "current account is not active")
	}
	currentEmail := normalizeAccountEmail(user.Email)
	if currentEmail == "" {
		return service.NewError(http.StatusBadRequest, "current email is required")
	}
	if newEmail == "" {
		return service.NewError(http.StatusBadRequest, "new email is required")
	}
	if newEmail == currentEmail {
		return service.NewError(http.StatusBadRequest, "new email must be different from current email")
	}

	existingUser, err := currentAccountGateway().FindByEmail(ctx, newEmail)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return nil
		}
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			return newAccountGatewayNotConfiguredServiceError(err)
		}
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to lookup target email", err)
	}
	if existingUser != nil && existingUser.ID != user.ID {
		return service.NewError(http.StatusConflict, "new email is already in use")
	}

	return nil
}

// validateEmailChangeFlow ensures the confirmation or cancellation flow carries
// the minimum state required to safely process the request.
func validateEmailChangeFlow(flow iamEmailFlowState) error {
	if strings.TrimSpace(flow.UserID) == "" {
		return service.NewError(http.StatusBadRequest, "email change account id is required")
	}
	if normalizeEmailScope(flow.OldEmail) == "" {
		return service.NewError(http.StatusBadRequest, "email change old email is required")
	}
	if normalizeEmailScope(flow.NewEmail) == "" {
		return service.NewError(http.StatusBadRequest, "email change new email is required")
	}
	if normalizeEmailScope(flow.OldEmail) == normalizeEmailScope(flow.NewEmail) {
		return service.NewError(http.StatusBadRequest, "email change old and new email must be different")
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
