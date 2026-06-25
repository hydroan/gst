package serviceemail

import (
	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// VerificationRequestService handles public requests that start the email
// verification flow for an eligible user account.
type VerificationRequestService struct {
	service.Base[*model.Empty, *modelemail.VerificationRequestReq, *modelemail.VerificationRequestRsp]
}

// Create starts an email verification flow and returns a generic acceptance
// message so callers cannot infer whether the target account exists.
func (s *VerificationRequestService) Create(ctx *types.ServiceContext, req *modelemail.VerificationRequestReq) (rsp *modelemail.VerificationRequestRsp, err error) {
	log := s.WithContext(ctx, ctx.GetPhase())
	rsp = &modelemail.VerificationRequestRsp{Msg: publicAcceptedMessage(iamEmailFlowKindVerification)}

	email := normalizeEmailScope(req.Email)
	if email == "" {
		return rsp, nil
	}

	if _, err = reserveEmailThrottle(ctx, iamEmailFlowKindVerification, emailThrottleRequest, email, 0); err != nil {
		if errors.Is(err, errEmailFlowThrottled) {
			return rsp, nil
		}
		log.Error("failed to reserve verification request throttle", err)
		return nil, errors.Wrap(err, "failed to reserve verification request throttle")
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
		log.Error("failed to load verification account", err)
		return nil, errors.Wrap(err, "failed to load verification account")
	}
	if !eligibleVerificationAccount(user, email) {
		return rsp, nil
	}

	token, flow, err := issueEmailFlow(ctx, iamEmailFlowKindVerification, iamEmailFlowState{
		UserID: user.ID,
		Email:  email,
	}, 0)
	if err != nil {
		log.Error("failed to issue verification flow", err)
		return nil, errors.Wrap(err, "failed to issue verification flow")
	}

	if err = dispatchEmail(ctx, verificationDelivery(token, flow)); err != nil {
		log.Error("failed to dispatch verification email", err)
		return nil, errors.Wrap(err, "failed to dispatch verification email")
	}

	return rsp, nil
}
