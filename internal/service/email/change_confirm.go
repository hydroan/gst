package serviceemail

import (
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modelemail "github.com/hydroan/gst/internal/model/email"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ChangeConfirmService handles the token confirmation step that finalizes a
// pending email change.
type ChangeConfirmService struct {
	service.Base[*modelemail.ChangeConfirm, *modelemail.ChangeConfirmReq, *modelemail.ChangeConfirmRsp]
}

// changeUpdateUser persists the confirmed email state for the target account.
var changeUpdateUser = func(ctx *types.ServiceContext, user *modeliamuser.User) error {
	return database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithoutHook().
		WithSelect("email", "email_verified", "email_verified_at", "last_email_changed_at").
		Update(user)
}

// Create consumes the confirmation token and updates the account email when the
// current database state still matches the pending change flow.
func (s *ChangeConfirmService) Create(ctx *types.ServiceContext, req *modelemail.ChangeConfirmReq) (rsp *modelemail.ChangeConfirmRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	flow, err := consumeEmailFlow(passwordResetContext(ctx), iamEmailFlowKindChangeConfirm, req.Token)
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

	user, err := changeLoadUserByID(ctx, flow.UserID)
	if err != nil {
		log.Error("failed to load email change confirmation user", err)
		return nil, errors.Wrap(err, "failed to load email change confirmation user")
	}

	currentEmail := normalizePasswordResetEmail(user.Email)
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

	existingUser, err := changeLookupUserByEmail(ctx, normalizeEmailScope(flow.NewEmail))
	if err != nil {
		if !errors.Is(err, errEmailUserNotFound) {
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

	if err = applyConfirmedEmailChange(user, flow.NewEmail, emailNow()); err != nil {
		log.Error("failed to apply confirmed email change", err)
		return nil, err
	}
	if err = changeUpdateUser(ctx, user); err != nil {
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

// validateEmailChangeFlow ensures the confirmation or cancellation flow carries
// the minimum state required to safely process the request.
func validateEmailChangeFlow(flow iamEmailFlowState) error {
	if strings.TrimSpace(flow.UserID) == "" {
		return errors.New("email change user id is required")
	}
	if normalizeEmailScope(flow.OldEmail) == "" {
		return errors.New("email change old email is required")
	}
	if normalizeEmailScope(flow.NewEmail) == "" {
		return errors.New("email change new email is required")
	}
	if normalizeEmailScope(flow.OldEmail) == normalizeEmailScope(flow.NewEmail) {
		return errors.New("email change old and new email must be different")
	}
	return nil
}

// applyConfirmedEmailChange mutates the in-memory user model to the confirmed
// email state before persistence.
func applyConfirmedEmailChange(user *modeliamuser.User, newEmail string, changedAt time.Time) error {
	if user == nil {
		return errors.New("email change user is required")
	}

	normalizedNewEmail := normalizeEmailScope(newEmail)
	if normalizedNewEmail == "" {
		return errors.New("email change new email is required")
	}

	verified := true
	changedAt = changedAt.UTC()
	user.Email = &normalizedNewEmail
	user.EmailVerified = &verified
	user.EmailVerifiedAt = &changedAt
	user.LastEmailChangedAt = &changedAt
	return nil
}
