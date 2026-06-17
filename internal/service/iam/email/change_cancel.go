package serviceiamemail

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	modeliamemail "github.com/hydroan/gst/internal/model/iam/email"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ChangeCancelService handles the token cancellation step that revokes a
// pending email change before confirmation happens.
type ChangeCancelService struct {
	service.Base[*modeliamemail.ChangeCancel, *modeliamemail.ChangeCancelReq, *modeliamemail.ChangeCancelRsp]
}

// Create consumes the cancellation token and records that matching
// confirmation tokens must no longer complete the email change.
func (s *ChangeCancelService) Create(ctx *types.ServiceContext, req *modeliamemail.ChangeCancelReq) (rsp *modeliamemail.ChangeCancelRsp, err error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	flow, err := consumeEmailFlow(passwordResetContext(ctx), iamEmailFlowKindChangeCancel, req.Token)
	if err != nil {
		if errors.Is(err, errEmailFlowNotFound) || errors.Is(err, errEmailFlowExpired) {
			return &modeliamemail.ChangeCancelRsp{
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

	user, err := changeLoadUserByID(ctx, flow.UserID)
	if err != nil {
		log.Error("failed to load email change cancellation user", err)
		return nil, errors.Wrap(err, "failed to load email change cancellation user")
	}

	currentEmail := normalizePasswordResetEmail(user.Email)
	switch currentEmail {
	case normalizeEmailScope(flow.NewEmail):
		return &modeliamemail.ChangeCancelRsp{
			Canceled: false,
			Msg:      "email already changed",
		}, nil
	case normalizeEmailScope(flow.OldEmail):
	default:
		return &modeliamemail.ChangeCancelRsp{
			Canceled: false,
			Msg:      "email change is no longer pending",
		}, nil
	}

	if err = markEmailChangeCanceled(ctx.Context(), flow); err != nil {
		log.Error("failed to mark email change as canceled", err)
		return nil, errors.Wrap(err, "failed to mark email change as canceled")
	}

	return &modeliamemail.ChangeCancelRsp{
		Canceled: true,
		Msg:      "email change canceled successfully",
	}, nil
}

// emailChangeCancellationKey returns the cache key that records a canceled
// change pair for the current user.
func emailChangeCancellationKey(userID, oldEmail, newEmail string) string {
	return strings.Join([]string{
		"iam",
		"email",
		"change",
		"canceled",
		strings.TrimSpace(userID),
		normalizeEmailScope(oldEmail),
		normalizeEmailScope(newEmail),
	}, ":")
}

// markEmailChangeCanceled stores a short-lived marker that blocks matching
// confirmation tokens from succeeding after cancellation.
func markEmailChangeCanceled(ctx context.Context, flow iamEmailFlowState) error {
	key := emailChangeCancellationKey(flow.UserID, flow.OldEmail, flow.NewEmail)
	now := emailNow()
	ttl := flow.ExpiresAt.Sub(now)
	if ttl <= 0 {
		ttl = defaultEmailPolicy.tokenTTL(iamEmailFlowKindChangeCancel)
	}

	record := emailThrottleRecord{
		Kind:        iamEmailFlowKindChangeCancel,
		Action:      emailThrottleRequest,
		Scope:       key,
		CreatedAt:   now,
		AvailableAt: now.Add(ttl),
	}
	if err := emailThrottleCache().WithContext(normalizeContext(ctx)).Set(key, record, ttl); err != nil {
		return errors.Wrap(err, "store email change cancellation marker")
	}

	return nil
}

// emailChangeCanceled reports whether a matching change pair has already been
// canceled and is still within the cancellation validity window.
func emailChangeCanceled(ctx context.Context, userID, oldEmail, newEmail string) (bool, error) {
	key := emailChangeCancellationKey(userID, oldEmail, newEmail)
	record, err := emailThrottleCache().WithContext(normalizeContext(ctx)).Get(key)
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return false, nil
		}
		return false, errors.Wrap(err, "load email change cancellation marker")
	}

	if record.AvailableAt.After(emailNow()) {
		return true, nil
	}
	if err = emailThrottleCache().WithContext(normalizeContext(ctx)).Delete(key); err != nil {
		return false, errors.Wrap(err, "delete expired email change cancellation marker")
	}
	return false, nil
}

// clearEmailChangeCancellation removes a stale cancellation marker so a fresh
// email change request for the same address pair can proceed normally.
func clearEmailChangeCancellation(ctx context.Context, userID, oldEmail, newEmail string) error {
	key := emailChangeCancellationKey(userID, oldEmail, newEmail)
	if err := emailThrottleCache().WithContext(normalizeContext(ctx)).Delete(key); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
		return errors.Wrap(err, "delete email change cancellation marker")
	}
	return nil
}
