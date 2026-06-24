package serviceemail

import (
	"github.com/cockroachdb/errors"
	modelemail "github.com/hydroan/gst/internal/model/email"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ChangeConfirmService handles the token confirmation step that finalizes a
// pending email change.
type ChangeConfirmService struct {
	service.Base[*modelemail.ChangeConfirm, *modelemail.ChangeConfirmReq, *modelemail.ChangeConfirmRsp]
}

// Create consumes the confirmation token and updates the account email when the
// current database state still matches the pending change flow.
func (s *ChangeConfirmService) Create(ctx *types.ServiceContext, req *modelemail.ChangeConfirmReq) (rsp *modelemail.ChangeConfirmRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	flow, err := consumeEmailFlow(emailServiceContext(ctx), iamEmailFlowKindChangeConfirm, req.Token)
	if err != nil {
		if errors.Is(err, errEmailFlowNotFound) || errors.Is(err, errEmailFlowExpired) {
			return &modelemail.ChangeConfirmRsp{
				Changed: false,
				Msg:     "invalid or expired email change token",
			}, nil
		}
		log.Error("failed to consume email change confirmation flow", err)
		return nil, errors.Wrap(err, "failed to consume email change confirmation flow")
	}
	if err = validateEmailChangeFlow(flow); err != nil {
		return nil, err
	}

	canceled, err := emailChangeCanceled(ctx.Context(), flow.UserID, flow.OldEmail, flow.NewEmail)
	if err != nil {
		log.Error("failed to check email change cancellation state", err)
		return nil, errors.Wrap(err, "failed to check email change cancellation state")
	}
	if canceled {
		return &modelemail.ChangeConfirmRsp{
			Changed: false,
			Msg:     "email change was canceled",
		}, nil
	}

	gateway := currentAccountGateway()
	user, err := gateway.GetByID(ctx, flow.UserID)
	if err != nil {
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			log.Error("email account gateway is not configured", err)
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		log.Error("failed to load email change confirmation account", err)
		return nil, errors.Wrap(err, "failed to load email change confirmation account")
	}
	if err = validAccountSnapshot(user, flow.UserID); err != nil {
		log.Error("email account gateway returned invalid email change confirmation account", err)
		return nil, newAccountGatewayInvalidAccountServiceError(err)
	}

	currentEmail := normalizeAccountEmail(user.Email)
	switch currentEmail {
	case normalizeEmailScope(flow.NewEmail):
		return &modelemail.ChangeConfirmRsp{
			Changed: true,
			Msg:     "email already changed",
		}, nil
	case normalizeEmailScope(flow.OldEmail):
	default:
		return &modelemail.ChangeConfirmRsp{
			Changed: false,
			Msg:     "email change can no longer be completed",
		}, nil
	}

	existingUser, err := gateway.FindByEmail(ctx, normalizeEmailScope(flow.NewEmail))
	if err != nil {
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			log.Error("email account gateway is not configured", err)
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		if !errors.Is(err, ErrAccountNotFound) {
			log.Error("failed to lookup target email for confirmation", err)
			return nil, errors.Wrap(err, "failed to lookup target email for confirmation")
		}
	}
	if existingUser != nil && existingUser.ID != user.ID {
		return &modelemail.ChangeConfirmRsp{
			Changed: false,
			Msg:     "email change can no longer be completed",
		}, nil
	}

	if err = gateway.ApplyEmailChange(ctx, user.ID, flow.NewEmail, emailNow()); err != nil {
		if errors.Is(err, ErrAccountGatewayNotConfigured) {
			log.Error("email account gateway is not configured", err)
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		log.Error("failed to persist confirmed email change", err)
		return nil, errors.Wrap(err, "failed to update email change state")
	}
	if err = clearEmailChangeCancellation(ctx.Context(), flow.UserID, flow.OldEmail, flow.NewEmail); err != nil {
		log.Error("failed to clear email change cancellation marker", err)
		return nil, errors.Wrap(err, "failed to clear email change cancellation marker")
	}

	return &modelemail.ChangeConfirmRsp{
		Changed: true,
		Msg:     "email changed successfully",
	}, nil
}
