package serviceemail

import (
	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// VerificationResendService handles public requests that resend verification
// emails for accounts that are still pending verification.
type VerificationResendService struct {
	service.Base[*model.Empty, *modelemail.VerificationResendReq, *modelemail.VerificationResendRsp]
}

// Create resends a verification email for an eligible account while keeping the
// response stable for unknown or already verified accounts.
func (s *VerificationResendService) Create(ctx *types.ServiceContext, req *modelemail.VerificationResendReq) (rsp *modelemail.VerificationResendRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())
	rsp = &modelemail.VerificationResendRsp{Msg: publicAcceptedMessage(iamEmailFlowKindVerification)}

	email := normalizeEmailScope(req.Email)
	if email == "" {
		return rsp, nil
	}

	if _, err = reserveEmailThrottle(ctx, iamEmailFlowKindVerification, emailThrottleResend, email, 0); err != nil {
		if errors.Is(err, errEmailFlowThrottled) {
			return rsp, nil
		}
		log.Error("failed to reserve verification resend throttle", err)
		return nil, errors.Wrap(err, "failed to reserve verification resend throttle")
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
		log.Error("failed to load verification resend account", err)
		return nil, errors.Wrap(err, "failed to load verification resend account")
	}
	if !eligibleVerificationAccount(user, email) {
		return rsp, nil
	}

	token, flow, err := issueEmailFlow(ctx, iamEmailFlowKindVerification, iamEmailFlowState{
		UserID: user.ID,
		Email:  email,
	}, 0)
	if err != nil {
		log.Error("failed to issue verification resend flow", err)
		return nil, errors.Wrap(err, "failed to issue verification resend flow")
	}

	if err = dispatchEmail(ctx, verificationDelivery(token, flow)); err != nil {
		log.Error("failed to dispatch verification resend email", err)
		return nil, errors.Wrap(err, "failed to dispatch verification resend email")
	}

	return rsp, nil
}
