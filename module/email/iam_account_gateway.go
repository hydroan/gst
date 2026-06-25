package email

import (
	"context"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceemail "github.com/hydroan/gst/internal/service/email"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/types"
	"golang.org/x/crypto/bcrypt"
)

// iamAccountGateway adapts the framework IAM user model for the built-in email
// module. It lives under module/email so copied email service code does not
// import the framework IAM user model, password hashing policy, or session store.
type iamAccountGateway struct{}

func (iamAccountGateway) FindByEmail(ctx *types.ServiceContext, email string) (*serviceemail.AccountSnapshot, error) {
	user, err := loadIAMUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return iamAccountSnapshot(user), nil
}

func (iamAccountGateway) GetByID(ctx *types.ServiceContext, userID string) (*serviceemail.AccountSnapshot, error) {
	user, err := loadIAMUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return iamAccountSnapshot(user), nil
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
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return serviceemail.ErrAccountAuthenticationFailed
	}
	return nil
}

func (iamAccountGateway) UpdatePassword(ctx *types.ServiceContext, userID, newPassword string) error {
	user, err := loadIAMUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := applyIAMPasswordUpdate(user, newPassword); err != nil {
		return err
	}
	return database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithoutHook().
		WithSelect("username", "password_hash", "must_change_password").
		Update(user)
}

func (iamAccountGateway) MarkEmailVerified(ctx *types.ServiceContext, userID string, verifiedAt time.Time) error {
	user := newIAMUserWithID(userID)
	applyIAMEmailVerification(user, verifiedAt)
	return database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithoutHook().
		WithSelect("email_verified", "email_verified_at").
		Update(user)
}

func (iamAccountGateway) ApplyEmailChange(ctx *types.ServiceContext, userID, newEmail string, changedAt time.Time) error {
	user := newIAMUserWithID(userID)
	if err := applyIAMEmailChange(user, newEmail, changedAt); err != nil {
		return err
	}
	return database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithoutHook().
		WithSelect("email", "email_verified", "email_verified_at", "last_email_changed_at").
		Update(user)
}

func (iamAccountGateway) InvalidateSessions(userID string) {
	serviceiamsession.InvalidateUserSessions(context.Background(), userID)
}

func loadIAMUserByEmail(ctx *types.ServiceContext, email string) (*modeliamuser.User, error) {
	users := make([]*modeliamuser.User, 0, 1)
	queryEmail := email
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).
		WithLimit(1).
		WithQuery(&modeliamuser.User{Email: &queryEmail}).
		List(&users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, serviceemail.ErrAccountNotFound
	}
	return users[0], nil
}

func loadIAMUserByID(ctx *types.ServiceContext, userID string) (*modeliamuser.User, error) {
	query := &modeliamuser.User{}
	query.ID = userID

	users := make([]*modeliamuser.User, 0, 1)
	if err := database.Database[*modeliamuser.User](ctx.DatabaseContext()).
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

func iamAccountSnapshot(user *modeliamuser.User) *serviceemail.AccountSnapshot {
	if user == nil {
		return nil
	}

	email := ""
	if user.Email != nil {
		email = *user.Email
	}

	return &serviceemail.AccountSnapshot{
		ID:            user.ID,
		Email:         email,
		Active:        iamUserActive(user),
		EmailVerified: user.EmailVerified != nil && *user.EmailVerified,
	}
}

func iamUserActive(user *modeliamuser.User) bool {
	return user != nil && (user.Status == "" || user.Status == modeliamuser.UserStatusActive)
}

func newIAMUserWithID(userID string) *modeliamuser.User {
	user := new(modeliamuser.User)
	user.ID = userID
	return user
}

func applyIAMPasswordUpdate(user *modeliamuser.User, newPassword string) error {
	if user == nil {
		return errors.New("password update account is required")
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.Wrap(err, "failed to process new password")
	}
	user.PasswordHash = string(hashedPassword)
	user.MustChangePassword = false
	return nil
}

func applyIAMEmailVerification(user *modeliamuser.User, verifiedAt time.Time) {
	verified := true
	verifiedAt = verifiedAt.UTC()
	user.EmailVerified = &verified
	user.EmailVerifiedAt = &verifiedAt
}

func applyIAMEmailChange(user *modeliamuser.User, newEmail string, changedAt time.Time) error {
	if user == nil {
		return errors.New("email change account is required")
	}

	normalizedNewEmail := normalizeEmailScope(newEmail)
	if normalizedNewEmail == "" {
		return errors.New("email change new email is required")
	}

	applyIAMEmailVerification(user, changedAt)
	user.Email = &normalizedNewEmail
	user.LastEmailChangedAt = user.EmailVerifiedAt
	return nil
}

func normalizeEmailScope(scope string) string {
	return strings.ToLower(strings.TrimSpace(scope))
}
