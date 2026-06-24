package serviceemail

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
)

// validateEmailChangeTarget ensures the current account can start an email
// change flow to the requested target address.
func validateEmailChangeTarget(ctx *types.ServiceContext, user *AccountSnapshot, newEmail string) error {
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return errors.New("current account is required")
	}
	if !user.Active {
		return errors.New("current account is not active")
	}
	currentEmail := normalizeAccountEmail(user.Email)
	if currentEmail == "" {
		return errors.New("current email is required")
	}
	if newEmail == "" {
		return errors.New("new email is required")
	}
	if newEmail == currentEmail {
		return errors.New("new email must be different from current email")
	}

	existingUser, err := currentAccountGateway().FindByEmail(ctx, newEmail)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return nil
		}
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			return newAccountGatewayNotConfiguredServiceError(err)
		}
		return errors.Wrap(err, "failed to lookup target email")
	}
	if existingUser != nil && existingUser.ID != user.ID {
		return errors.New("new email is already in use")
	}

	return nil
}

// validateEmailChangeFlow ensures the confirmation or cancellation flow carries
// the minimum state required to safely process the request.
func validateEmailChangeFlow(flow iamEmailFlowState) error {
	if strings.TrimSpace(flow.UserID) == "" {
		return errors.New("email change account id is required")
	}
	if normalizeEmailScope(flow.OldEmail) == "" {
		return errors.New("email change old email is required")
	}
	if normalizeEmailScope(flow.NewEmail) == "" {
		return errors.New("email change new email is required")
	}
	if normalizeEmailScope(flow.OldEmail) == normalizeEmailScope(flow.NewEmail) {
		return errors.New("email change old and new email must be different")
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
