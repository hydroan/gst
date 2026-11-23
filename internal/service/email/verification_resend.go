package serviceemail

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"go.uber.org/zap"
)

// VerificationResendService handles public requests that resend verification
// emails for accounts that are still pending verification.
type VerificationResendService struct {
	service.Base[*model.Empty, *modelemail.VerificationResendReq, *modelemail.VerificationResendRsp]
}

// Create resends a verification email for an eligible account while keeping the
// response stable for unknown or already verified accounts. An address that is
// no email address (see isEmailAddress) is refused before any of it: the
// refusal reads the request alone and tells nothing of an account.
func (s *VerificationResendService) Create(ctx *gst.ServiceContext, req *modelemail.VerificationResendReq) (rsp *modelemail.VerificationResendRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())
	rsp = &modelemail.VerificationResendRsp{Msg: publicAcceptedMessage(iamEmailFlowKindVerification)}

	email := normalizeEmailScope(req.Email)
	if !isEmailAddress(email) {
		return nil, gst.NewError(http.StatusBadRequest, emailMustBeValidEmailAddressMsg)
	}

	if _, err = reserveEmailThrottle(ctx, iamEmailFlowKindVerification, emailThrottleResend, email, 0); err != nil {
		if errors.Is(err, errEmailFlowThrottled) {
			return rsp, nil
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToReserveVerificationResendThrottleMsg, err)
	}

	user, err := currentAccountGateway().FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return rsp, nil
		}
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			log.Errorz("email account gateway is not configured", zap.Error(err))
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToLoadVerificationResendAccountMsg, err)
	}
	if !eligibleVerificationAccount(user, email) {
		return rsp, nil
	}

	token, flow, err := issueEmailFlow(ctx, iamEmailFlowKindVerification, iamEmailFlowState{
		UserID: user.ID,
		Email:  email,
	})
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToIssueVerificationResendFlowMsg, err)
	}

	if err = dispatchEmail(ctx, verificationDelivery(token, flow)); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToDispatchVerificationResendEmailMsg, err)
	}

	return rsp, nil
}
