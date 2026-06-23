package serviceemail

import (
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

// Create revalidates the current user and reissues the confirmation email for
// the target new email address.
func (s *ChangeResendService) Create(ctx *types.ServiceContext, req *modelemail.ChangeResendReq) (rsp *modelemail.ChangeResendRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())
	if ctx == nil || strings.TrimSpace(ctx.UserID) == "" {
		return nil, errors.New("authentication required")
	}

	user, err := changeLoadUserByID(ctx, ctx.UserID)
	if err != nil {
		log.Error("failed to load email change resend user", err)
		return nil, errors.Wrap(err, "failed to load current user")
	}

	newEmail := normalizeEmailScope(req.NewEmail)
	if err = validateEmailChangeTarget(ctx, user, newEmail); err != nil {
		log.Error("failed to validate email change resend target", err)
		return nil, err
	}

	if _, err = reserveEmailThrottle(ctx.Context(), iamEmailFlowKindChangeConfirm, emailThrottleResend, newEmail, 0); err != nil {
		if errors.Is(err, errEmailFlowThrottled) {
			return &modelemail.ChangeResendRsp{Msg: "email change confirmation resent successfully"}, nil
		}
		log.Error("failed to reserve email change resend throttle", err)
		return nil, errors.Wrap(err, "failed to reserve email change resend throttle")
	}

	confirmToken, confirmFlow, err := issueEmailFlow(ctx.Context(), iamEmailFlowKindChangeConfirm, iamEmailFlowState{
		UserID:   user.ID,
		OldEmail: normalizePasswordResetEmail(user.Email),
		NewEmail: newEmail,
		Email:    newEmail,
	}, 0)
	if err != nil {
		log.Error("failed to issue email change resend flow", err)
		return nil, errors.Wrap(err, "failed to issue email change resend flow")
	}
	if err = dispatchEmail(ctx.Context(), changeConfirmDelivery(confirmToken, confirmFlow)); err != nil {
		log.Error("failed to dispatch email change resend confirmation", err)
		return nil, errors.Wrap(err, "failed to dispatch email change resend confirmation")
	}

	return &modelemail.ChangeResendRsp{Msg: "email change confirmation resent successfully"}, nil
}
