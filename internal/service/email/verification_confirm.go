package serviceemail

import (
	"strings"

	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// VerificationConfirmService handles the token confirmation step that finalizes
// the email verification flow.
type VerificationConfirmService struct {
	service.Base[*model.Empty, *modelemail.VerificationConfirmReq, *modelemail.VerificationConfirmRsp]
}

// Create consumes the one-time verification token and marks the corresponding
// email address as verified when the current account state still matches.
func (s *VerificationConfirmService) Create(ctx *types.ServiceContext, req *modelemail.VerificationConfirmReq) (rsp *modelemail.VerificationConfirmRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	flow, err := consumeEmailFlow(emailServiceContext(ctx), iamEmailFlowKindVerification, req.Token)
	if err != nil {
		if errors.Is(err, errEmailFlowNotFound) || errors.Is(err, errEmailFlowExpired) {
			return &modelemail.VerificationConfirmRsp{
				Verified: false,
				Msg:      "invalid or expired verification token",
			}, nil
		}
		log.Error("failed to consume verification flow", err)
		return nil, errors.Wrap(err, "failed to consume verification flow")
	}
	if strings.TrimSpace(flow.UserID) == "" {
		return nil, errors.New("verification account id is required")
	}

	gateway := currentAccountGateway()
	user, err := gateway.GetByID(ctx, flow.UserID)
	if err != nil {
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			log.Error("email account gateway is not configured", err)
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		log.Error("failed to load verification account", err)
		return nil, errors.Wrap(err, "failed to load verification account")
	}
	if err = validAccountSnapshot(user, flow.UserID); err != nil {
		log.Error("email account gateway returned invalid verification account", err)
		return nil, newAccountGatewayInvalidAccountServiceError(err)
	}
	if normalizeAccountEmail(user.Email) != normalizeEmailScope(flow.Email) {
		return &modelemail.VerificationConfirmRsp{
			Verified: false,
			Msg:      "invalid or expired verification token",
		}, nil
	}
	if accountEmailVerified(user) {
		return &modelemail.VerificationConfirmRsp{
			Verified: true,
			Msg:      "email already verified",
		}, nil
	}

	if err = gateway.MarkEmailVerified(ctx, user.ID, emailNow()); err != nil {
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			log.Error("email account gateway is not configured", err)
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		log.Error("failed to update verification account", err)
		return nil, errors.Wrap(err, "failed to update email verification state")
	}

	return &modelemail.VerificationConfirmRsp{
		Verified: true,
		Msg:      "email verified successfully",
	}, nil
}
