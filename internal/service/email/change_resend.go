package serviceemail

import (
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ChangeResendService handles authenticated requests that resend confirmation
// emails for an in-progress email change flow.
type ChangeResendService struct {
	service.Base[*modelemail.ChangeResend, *modelemail.ChangeResendReq, *modelemail.ChangeResendRsp]
}

// Create revalidates the current account and reissues the confirmation email for
// the target new email address.
func (s *ChangeResendService) Create(ctx *types.ServiceContext, req *modelemail.ChangeResendReq) (rsp *modelemail.ChangeResendRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())
	if ctx == nil || strings.TrimSpace(ctx.UserID()) == "" {
		return nil, service.NewError(http.StatusBadRequest, "authentication required")
	}

	user, err := currentAccountGateway().GetByID(ctx, ctx.UserID())
	if err != nil {
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			log.Error("email account gateway is not configured", err)
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load current account", err)
	}
	if err = validAccountSnapshot(user, ctx.UserID()); err != nil {
		log.Error("email account gateway returned invalid email change resend account", err)
		return nil, newAccountGatewayInvalidAccountServiceError(err)
	}

	newEmail := normalizeEmailScope(req.NewEmail)
	if err = validateEmailChangeTarget(ctx, user, newEmail); err != nil {
		log.Error("failed to validate email change resend target", err)
		return nil, err
	}

	if _, err = reserveEmailThrottle(ctx, iamEmailFlowKindChangeConfirm, emailThrottleResend, newEmail, 0); err != nil {
		if errors.Is(err, errEmailFlowThrottled) {
			return &modelemail.ChangeResendRsp{Msg: "email change confirmation resent successfully"}, nil
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to reserve email change resend throttle", err)
	}

	confirmToken, confirmFlow, err := issueEmailFlow(ctx, iamEmailFlowKindChangeConfirm, iamEmailFlowState{
		UserID:   user.ID,
		OldEmail: normalizeAccountEmail(user.Email),
		NewEmail: newEmail,
		Email:    newEmail,
	})
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to issue email change resend flow", err)
	}
	if err = dispatchEmail(ctx, changeConfirmDelivery(confirmToken, confirmFlow)); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to dispatch email change resend confirmation", err)
	}

	return &modelemail.ChangeResendRsp{Msg: "email change confirmation resent successfully"}, nil
}
