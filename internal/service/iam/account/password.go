package serviceiamaccount

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	"golang.org/x/crypto/bcrypt"
)

const minAccountPasswordLength = 6

func validateChangePasswordInput(req *modeliamaccount.ChangePasswordReq) error {
	if req == nil {
		return errors.New("change password request is required")
	}
	if req.OldPassword == "" {
		return errors.New("old password is required")
	}
	return validateNewAccountPassword(req.NewPassword)
}

func validateResetPasswordInput(req *modeliamaccount.ResetPasswordReq) error {
	if req == nil {
		return errors.New("reset password request is required")
	}
	if req.UserID == "" {
		return errors.New("user_id is required")
	}
	return validateNewAccountPassword(req.NewPassword)
}

func validateNewAccountPassword(password string) error {
	if password == "" {
		return errors.New("new password is required")
	}
	if len(password) < minAccountPasswordLength {
		return errors.New("password must be at least 6 characters long")
	}
	return nil
}

// NewPasswordCredential creates a password credential for the given IAM user.
func NewPasswordCredential(userID, password string, mustChangePassword bool) (*modeliamaccount.PasswordCredential, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}

	credential := &modeliamaccount.PasswordCredential{UserID: userID}
	if err := ApplyPasswordCredentialUpdate(credential, password, mustChangePassword); err != nil {
		return nil, err
	}
	return credential, nil
}

// LoadPasswordCredential loads the password credential owned by the given IAM user.
func LoadPasswordCredential(ctx context.Context, userID string) (*modeliamaccount.PasswordCredential, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}

	credentials := make([]*modeliamaccount.PasswordCredential, 0, 1)
	if err := database.Database[*modeliamaccount.PasswordCredential](ctx).
		WithLimit(1).
		WithQuery(&modeliamaccount.PasswordCredential{UserID: userID}).
		List(&credentials); err != nil {
		return nil, err
	}
	if len(credentials) == 0 {
		return nil, database.ErrRecordNotFound
	}
	return credentials[0], nil
}

// VerifyPasswordCredential verifies a plaintext password against a credential hash.
func VerifyPasswordCredential(credential *modeliamaccount.PasswordCredential, password string) error {
	if credential == nil {
		return errors.New("password credential is required")
	}
	return bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(password))
}

// ApplyPasswordCredentialUpdate replaces the credential hash and password-change state.
func ApplyPasswordCredentialUpdate(credential *modeliamaccount.PasswordCredential, newPassword string, mustChangePassword bool) error {
	if credential == nil {
		return errors.New("password credential is required")
	}

	passwordHash, err := hashAccountPassword(newPassword)
	if err != nil {
		return err
	}
	now := time.Now()
	credential.PasswordHash = passwordHash
	credential.MustChangePassword = mustChangePassword
	credential.PasswordChangedAt = &now
	return nil
}

func hashAccountPassword(password string) (string, error) {
	if err := validateNewAccountPassword(password); err != nil {
		return "", err
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", errors.Wrap(err, "hash password")
	}
	return string(hashedPassword), nil
}
