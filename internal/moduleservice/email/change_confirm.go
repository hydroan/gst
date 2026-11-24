package serviceemail

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	modelemail "github.com/hydroan/gst/internal/modulemodel/email"
	"github.com/hydroan/gst/service"
	"go.uber.org/zap"
)

// ChangeConfirmService handles the token confirmation step that finalizes a
// pending email change.
type ChangeConfirmService struct {
	service.Base[*modelemail.ChangeConfirm, *modelemail.ChangeConfirmReq, *modelemail.ChangeConfirmRsp]
}

// Create consumes the confirmation token and updates the account email when the
// current database state still matches the pending change flow.
func (s *ChangeConfirmService) Create(ctx *gst.ServiceContext, req *modelemail.ChangeConfirmReq) (rsp *modelemail.ChangeConfirmRsp, err error) {
	log := s.WithContext(ctx, ctx.Phase())

	flow, err := consumeEmailFlow(emailServiceContext(ctx), iamEmailFlowKindChangeConfirm, req.Token)
	if err != nil {
		if errors.Is(err, errEmailFlowNotFound) || errors.Is(err, errEmailFlowExpired) {
			return &modelemail.ChangeConfirmRsp{
				Changed: false,
				Msg:     "invalid or expired email change token",
			}, nil
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToConsumeEmailChangeConfirmationFlowMsg, err)
	}
	if err = validateEmailChangeFlow(flow); err != nil {
		return nil, err
	}

	canceled, err := emailChangeCanceled(ctx, flow.UserID, flow.OldEmail, flow.NewEmail)
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToCheckEmailChangeCancellationStateMsg, err)
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
			log.Errorz("email account gateway is not configured", zap.Error(err))
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToLoadEmailChangeConfirmationAccountMsg, err)
	}
	if err = validAccountSnapshot(user, flow.UserID); err != nil {
		log.Errorz("email account gateway returned invalid email change confirmation account", zap.Error(err))
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
			log.Errorz("email account gateway is not configured", zap.Error(err))
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		if !errors.Is(err, ErrAccountNotFound) {
			return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToLookUpTargetEmailMsg, err)
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
			log.Errorz("email account gateway is not configured", zap.Error(err))
			return nil, newAccountGatewayNotConfiguredServiceError(err)
		}
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToUpdateEmailChangeStateMsg, err)
	}
	if err = clearEmailChangeCancellation(ctx, flow.UserID, flow.OldEmail, flow.NewEmail); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, failedToClearEmailChangeCancellationMarkerMsg, err)
	}

	return &modelemail.ChangeConfirmRsp{
		Changed: true,
		Msg:     "email changed successfully",
	}, nil
}
