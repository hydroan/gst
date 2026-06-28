package email

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceemail "github.com/hydroan/gst/internal/service/email"
	serviceiamaccount "github.com/hydroan/gst/internal/service/iam/account"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/types"
)

// iamAccountGateway adapts the framework IAM user model for the built-in email
// module. It lives under module/email so copied email service code does not
// import the framework IAM user model, password hashing policy, or session store.
type iamAccountGateway struct{}

func (iamAccountGateway) FindByEmail(ctx *types.ServiceContext, email string) (*serviceemail.AccountSnapshot, error) {
	identity, err := serviceiamaccount.LoadEmailIdentityByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, serviceemail.ErrAccountNotFound
		}
		return nil, err
	}
	user, err := loadIAMUserByID(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	return iamAccountSnapshot(user, identity), nil
}

func (iamAccountGateway) GetByID(ctx *types.ServiceContext, userID string) (*serviceemail.AccountSnapshot, error) {
	user, err := loadIAMUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	identity, err := serviceiamaccount.LoadEmailIdentity(ctx, userID)
	if err != nil {
		if !errors.Is(err, database.ErrRecordNotFound) {
			return nil, err
		}
		identity = nil
	}
	return iamAccountSnapshot(user, identity), nil
}

func (iamAccountGateway) VerifyPassword(ctx *types.ServiceContext, userID, password string) error {
	user, err := loadIAMUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, serviceemail.ErrAccountNotFound) {
			return serviceemail.ErrAccountAuthenticationFailed
		}
		return err
	}
	if !iamUserActive(user) {
		return serviceemail.ErrAccountAuthenticationFailed
	}
	credential, err := serviceiamaccount.LoadPasswordCredential(ctx, user.ID)
	if err != nil {
		return serviceemail.ErrAccountAuthenticationFailed
	}
	if err = serviceiamaccount.VerifyPasswordCredential(credential, password); err != nil {
		return serviceemail.ErrAccountAuthenticationFailed
	}
	return nil
}

func (iamAccountGateway) UpdatePassword(ctx *types.ServiceContext, userID, newPassword string) error {
	if _, err := loadIAMUserByID(ctx, userID); err != nil {
		return err
	}
	credential, err := serviceiamaccount.LoadPasswordCredential(ctx, userID)
	if err != nil {
		return err
	}
	if err := applyIAMPasswordUpdate(credential, newPassword); err != nil {
		return err
	}
	return database.Database[*modeliamaccount.PasswordCredential](ctx).
		WithoutHook().
		WithSelect("user_id", "password_hash", "must_change_password", "password_changed_at").
		Update(credential)
}

func (iamAccountGateway) MarkEmailVerified(ctx *types.ServiceContext, userID string, verifiedAt time.Time) error {
	identity, err := serviceiamaccount.LoadEmailIdentity(ctx, userID)
	if err != nil {
		return err
	}
	if err := serviceiamaccount.ApplyEmailIdentityVerification(identity, verifiedAt); err != nil {
		return err
	}
	return database.Database[*modeliamaccount.EmailIdentity](ctx).
		WithoutHook().
		WithSelect("user_id", "verified_at").
		Update(identity)
}

func (iamAccountGateway) ApplyEmailChange(ctx *types.ServiceContext, userID, newEmail string, changedAt time.Time) error {
	identity, err := serviceiamaccount.LoadEmailIdentity(ctx, userID)
	if err != nil {
		if !errors.Is(err, database.ErrRecordNotFound) {
			return err
		}
		identity = &modeliamaccount.EmailIdentity{UserID: userID}
	}
	if err := applyIAMEmailChange(identity, newEmail, changedAt); err != nil {
		return err
	}
	if identity.ID == "" {
		return database.Database[*modeliamaccount.EmailIdentity](ctx).Create(identity)
	}
	return database.Database[*modeliamaccount.EmailIdentity](ctx).
		WithoutHook().
		WithSelect("user_id", "email", "normalized_email", "verified_at", "last_changed_at").
		Update(identity)
}

func (iamAccountGateway) InvalidateSessions(userID string) {
	serviceiamsession.InvalidateUserSessions(context.Background(), userID)
}

func loadIAMUserByID(ctx *types.ServiceContext, userID string) (*modeliamuser.User, error) {
	query := &modeliamuser.User{}
	query.ID = userID

	users := make([]*modeliamuser.User, 0, 1)
	if err := database.Database[*modeliamuser.User](ctx).
		WithLimit(1).
		WithQuery(query).
		List(&users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, serviceemail.ErrAccountNotFound
	}
	return users[0], nil
}

func iamAccountSnapshot(user *modeliamuser.User, identity *modeliamaccount.EmailIdentity) *serviceemail.AccountSnapshot {
	if user == nil {
		return nil
	}

	email := ""
	emailVerified := false
	if identity != nil {
		email = identity.Email
		emailVerified = identity.VerifiedAt != nil
	}

	return &serviceemail.AccountSnapshot{
		ID:            user.ID,
		Email:         email,
		Active:        iamUserActive(user),
		EmailVerified: emailVerified,
	}
}

func iamUserActive(user *modeliamuser.User) bool {
	return user != nil && (user.Status == "" || user.Status == modeliamuser.UserStatusActive)
}

func applyIAMPasswordUpdate(credential *modeliamaccount.PasswordCredential, newPassword string) error {
	if credential == nil {
		return errors.New("password update account is required")
	}
	return serviceiamaccount.ApplyPasswordCredentialUpdate(credential, newPassword, false)
}

func applyIAMEmailChange(identity *modeliamaccount.EmailIdentity, newEmail string, changedAt time.Time) error {
	if identity == nil {
		return errors.New("email change account is required")
	}
	return serviceiamaccount.ApplyEmailIdentityChange(identity, newEmail, changedAt)
}
