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
	log := s.WithServiceContext(ctx, ctx.GetPhase())
	rsp = &modelemail.VerificationRequestRsp{Msg: publicAcceptedMessage(iamEmailFlowKindVerification)}

	email := normalizeEmailScope(req.Email)
	if email == "" {
		return rsp, nil
	}

	if _, err = reserveEmailThrottle(ctx.Context(), iamEmailFlowKindVerification, emailThrottleRequest, email, 0); err != nil {
		if errors.Is(err, errEmailFlowThrottled) {
			return rsp, nil
		}
		log.Error("failed to reserve verification request throttle", err)
		return nil, errors.Wrap(err, "failed to reserve verification request throttle")
	}

	user, err := currentUserProvider().FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return rsp, nil
		}
		if errors.Is(err, ErrUserProviderNotConfigured) {
			log.Error("email user provider is not configured", err)
			return nil, newUserProviderNotConfiguredServiceError(err)
		}
		log.Error("failed to load verification user", err)
		return nil, errors.Wrap(err, "failed to load verification user")
	}
	if !eligibleVerificationUser(user, email) {
		return rsp, nil
	}

	token, flow, err := issueEmailFlow(ctx.Context(), iamEmailFlowKindVerification, iamEmailFlowState{
		UserID: user.ID,
		Email:  email,
	}, 0)
	if err != nil {
		log.Error("failed to issue verification flow", err)
		return nil, errors.Wrap(err, "failed to issue verification flow")
	}

	if err = dispatchEmail(ctx.Context(), verificationDelivery(token, flow)); err != nil {
		log.Error("failed to dispatch verification email", err)
		return nil, errors.Wrap(err, "failed to dispatch verification email")
	}

	return rsp, nil
}

// eligibleVerificationUser ensures the verification flow is only sent to an
// active account whose current email still matches the normalized request email
// and has not already been verified.
func eligibleVerificationUser(user *UserSnapshot, email string) bool {
	if user == nil || user.ID == "" {
		return false
	}
	if normalizeUserEmail(user.Email) != email {
		return false
	}
	if !user.Active {
		return false
	}
	return !user.EmailVerified
}

// verificationDelivery builds the email payload for the verification sender.
func verificationDelivery(token string, flow iamEmailFlowState) emailDelivery {
	return emailDelivery{
		To:       flow.Email,
		Subject:  "Email verification",
		Template: "iam/email/verification",
		Data: map[string]any{
			"token":      token,
			"user_id":    flow.UserID,
			"email":      flow.Email,
			"expires_at": flow.ExpiresAt,
		},
	}
}

// userEmailVerified safely returns the email verification flag for a user snapshot.
func userEmailVerified(user *UserSnapshot) bool {
	return user != nil && user.EmailVerified
}
