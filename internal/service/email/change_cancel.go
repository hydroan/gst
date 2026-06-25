package serviceemail

import (
	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ChangeCancelService handles the token cancellation step that revokes a
// pending email change before confirmation happens.
type ChangeCancelService struct {
	service.Base[*modelemail.ChangeCancel, *modelemail.ChangeCancelReq, *modelemail.ChangeCancelRsp]
}

// Create consumes the cancellation token and records that matching
// confirmation tokens must no longer complete the email change.
func (s *ChangeCancelService) Create(ctx *types.ServiceContext, req *modelemail.ChangeCancelReq) (rsp *modelemail.ChangeCancelRsp, err error) {
	log := s.WithContext(ctx, ctx.GetPhase())

	flow, err := consumeEmailFlow(emailServiceContext(ctx), iamEmailFlowKindChangeCancel, req.Token)
	if err != nil {
		if errors.Is(err, errEmailFlowNotFound) || errors.Is(err, errEmailFlowExpired) {
			return &modelemail.ChangeCancelRsp{
				Canceled: false,
				Msg:      "invalid or expired email change cancellation token",
			}, nil
		}
		log.Error("failed to consume email change cancellation flow", err)
		return nil, errors.Wrap(err, "failed to consume email change cancellation flow")
	}
	if err = validateEmailChangeFlow(flow); err != nil {
		return nil, err
	}

	user, err := currentAccountGateway().GetByID(ctx, flow.UserID)
	if err != nil {
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			log.Error("email account gateway is not configured", err)
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		log.Error("failed to load email change cancellation account", err)
		return nil, errors.Wrap(err, "failed to load email change cancellation account")
	}
	if err = validAccountSnapshot(user, flow.UserID); err != nil {
		log.Error("email account gateway returned invalid email change cancellation account", err)
		return nil, newAccountGatewayInvalidAccountServiceError(err)
	}

	currentEmail := normalizeAccountEmail(user.Email)
	switch currentEmail {
	case normalizeEmailScope(flow.NewEmail):
		return &modelemail.ChangeCancelRsp{
			Canceled: false,
			Msg:      "email already changed",
		}, nil
	case normalizeEmailScope(flow.OldEmail):
	default:
		return &modelemail.ChangeCancelRsp{
			Canceled: false,
			Msg:      "email change is no longer pending",
		}, nil
	}

	if err = markEmailChangeCanceled(ctx, flow); err != nil {
		log.Error("failed to mark email change as canceled", err)
		return nil, errors.Wrap(err, "failed to mark email change as canceled")
	}

	return &modelemail.ChangeCancelRsp{
		Canceled: true,
		Msg:      "email change canceled successfully",
	}, nil
}
