package serviceiamaccount

import (
	"context"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
)

// NewEmailIdentity creates the primary email identity for the given IAM user.
func NewEmailIdentity(userID, email string) (*modeliamaccount.EmailIdentity, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}

	identity := &modeliamaccount.EmailIdentity{UserID: userID}
	if err := ApplyEmailIdentityChange(identity, email, time.Time{}); err != nil {
		return nil, err
	}
	identity.VerifiedAt = nil
	identity.LastChangedAt = nil
	return identity, nil
}

// LoadEmailIdentity loads the primary email identity owned by the given IAM user.
func LoadEmailIdentity(ctx context.Context, userID string) (*modeliamaccount.EmailIdentity, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}

	identities := make([]*modeliamaccount.EmailIdentity, 0, 1)
	if err := database.Database[*modeliamaccount.EmailIdentity](ctx).
		WithLimit(1).
		WithQuery(&modeliamaccount.EmailIdentity{UserID: userID}).
		List(&identities); err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		return nil, database.ErrRecordNotFound
	}
	return identities[0], nil
}

// LoadEmailIdentityByEmail loads the primary email identity for a normalized email address.
func LoadEmailIdentityByEmail(ctx context.Context, email string) (*modeliamaccount.EmailIdentity, error) {
	normalizedEmail := NormalizeEmailIdentity(email)
	if normalizedEmail == "" {
		return nil, errors.New("email is required")
	}

	identities := make([]*modeliamaccount.EmailIdentity, 0, 1)
	if err := database.Database[*modeliamaccount.EmailIdentity](ctx).
		WithLimit(1).
		WithQuery(&modeliamaccount.EmailIdentity{NormalizedEmail: normalizedEmail}).
		List(&identities); err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		return nil, database.ErrRecordNotFound
	}
	return identities[0], nil
}

// ApplyEmailIdentityVerification marks an email identity as verified.
func ApplyEmailIdentityVerification(identity *modeliamaccount.EmailIdentity, verifiedAt time.Time) error {
	if identity == nil {
		return errors.New("email identity is required")
	}
	verifiedAt = verifiedAt.UTC()
	identity.VerifiedAt = &verifiedAt
	return nil
}

// ApplyEmailIdentityChange replaces the email address and marks the new address as verified.
func ApplyEmailIdentityChange(identity *modeliamaccount.EmailIdentity, newEmail string, changedAt time.Time) error {
	if identity == nil {
		return errors.New("email identity is required")
	}

	displayEmail := strings.TrimSpace(newEmail)
	normalizedEmail := NormalizeEmailIdentity(newEmail)
	if normalizedEmail == "" {
		return errors.New("email is required")
	}

	identity.Email = displayEmail
	identity.NormalizedEmail = normalizedEmail
	if !changedAt.IsZero() {
		changedAt = changedAt.UTC()
		identity.VerifiedAt = &changedAt
		identity.LastChangedAt = &changedAt
	}
	return nil
}

// NormalizeEmailIdentity normalizes an email address for exact identity lookup.
func NormalizeEmailIdentity(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
