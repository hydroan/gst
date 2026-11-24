package serviceemail

import (
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
)

// validateEmailChangeTarget ensures the current account can start an email
// change flow to the requested target address.
func validateEmailChangeTarget(ctx *gst.ServiceContext, user *AccountSnapshot, newEmail string) error {
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return gst.NewError(http.StatusBadRequest, currentAccountRequiredMsg)
	}
	if !user.Active {
		return gst.NewError(http.StatusBadRequest, currentAccountNotActiveMsg)
	}
	currentEmail := normalizeAccountEmail(user.Email)
	if currentEmail == "" {
		return gst.NewError(http.StatusBadRequest, currentEmailRequiredMsg)
	}
	if newEmail == "" {
		return gst.NewError(http.StatusBadRequest, newEmailRequiredMsg)
	}
	if !isEmailAddress(newEmail) {
		return gst.NewError(http.StatusBadRequest, newEmailMustBeValidEmailAddressMsg)
	}
	if newEmail == currentEmail {
		return gst.NewError(http.StatusBadRequest, newEmailMustBeDifferentFromCurrentEmailMsg)
	}

	existingUser, err := currentAccountGateway().FindByEmail(ctx, newEmail)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return nil
		}
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			return newAccountGatewayNotConfiguredServiceError(err)
		}
		return gst.NewErrorWithCause(http.StatusInternalServerError, failedToLookUpTargetEmailMsg, err)
	}
	if existingUser != nil && existingUser.ID != user.ID {
		return gst.NewError(http.StatusConflict, newEmailAlreadyInUseMsg)
	}

	return nil
}

// validateEmailChangeFlow ensures the confirmation or cancellation flow carries
// the minimum state required to safely process the request.
func validateEmailChangeFlow(flow iamEmailFlowState) error {
	if strings.TrimSpace(flow.UserID) == "" {
		return gst.NewError(http.StatusBadRequest, emailChangeAccountIDRequiredMsg)
	}
	if normalizeEmailScope(flow.OldEmail) == "" {
		return gst.NewError(http.StatusBadRequest, emailChangeOldEmailRequiredMsg)
	}
	if normalizeEmailScope(flow.NewEmail) == "" {
		return gst.NewError(http.StatusBadRequest, emailChangeNewEmailRequiredMsg)
	}
	if normalizeEmailScope(flow.OldEmail) == normalizeEmailScope(flow.NewEmail) {
		return gst.NewError(http.StatusBadRequest, emailChangeOldAndNewEmailMustBeDifferentMsg)
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
