package serviceemail

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
)

// emailChangeCancellationKey returns the cache key that records a canceled
// change pair for the current account.
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
