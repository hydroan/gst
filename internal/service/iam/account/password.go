package serviceiamaccount

import (
	"context"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/service"
	"golang.org/x/crypto/bcrypt"
)

const minAccountPasswordLength = 6

func validateChangePasswordInput(req *modeliamaccount.ChangePasswordReq) error {
	if req == nil {
		return service.NewError(http.StatusBadRequest, "change password request is required")
	}
	if req.OldPassword == "" {
		return service.NewError(http.StatusBadRequest, "old password is required")
	}
	return validateNewAccountPassword(req.NewPassword)
}

func validateResetPasswordInput(req *modeliamaccount.ResetPasswordReq) error {
	if req == nil {
		return service.NewError(http.StatusBadRequest, "reset password request is required")
	}
	if req.UserID == "" {
		return service.NewError(http.StatusBadRequest, "user_id is required")
	}
	return validateNewAccountPassword(req.NewPassword)
}

func validateNewAccountPassword(password string) error {
	if password == "" {
		return service.NewError(http.StatusBadRequest, "new password is required")
	}
	if len(password) < minAccountPasswordLength {
		return service.NewError(http.StatusBadRequest, "password must be at least 6 characters long")
	}
	return nil
}

// NewPasswordCredential creates a password credential for the given IAM user.
func NewPasswordCredential(ctx context.Context, userID, password string, mustChangePassword bool) (*modeliamaccount.PasswordCredential, error) {
	if userID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user_id is required")
	}

	credential := &modeliamaccount.PasswordCredential{UserID: userID}
	if err := ApplyPasswordCredentialUpdate(ctx, credential, password, mustChangePassword); err != nil {
		return nil, err
	}
	return credential, nil
}

// LoadPasswordCredential loads the password credential owned by the given IAM user.
func LoadPasswordCredential(ctx context.Context, userID string) (*modeliamaccount.PasswordCredential, error) {
	if userID == "" {
		return nil, service.NewError(http.StatusBadRequest, "user_id is required")
	}

	credentials := make([]*modeliamaccount.PasswordCredential, 0, 1)
	if err := database.Database[*modeliamaccount.PasswordCredential](ctx).
		WithLimit(1).
		WithQuery(&modeliamaccount.PasswordCredential{UserID: userID}).
		List(&credentials); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load password credential", err)
	}
	if len(credentials) == 0 {
		return nil, database.ErrRecordNotFound
	}
	return credentials[0], nil
}

// VerifyPasswordCredential verifies a plaintext password against a credential hash.
// The bcrypt comparison is deliberately slow (tens of milliseconds) to resist
// brute-force attacks, so it runs inside an iam.VerifyPassword span to keep that
// cost visible in request traces. A password mismatch is an expected business
// outcome and never marks the span as failed; only unexpected verification
// errors are recorded as span errors.
func VerifyPasswordCredential(ctx context.Context, credential *modeliamaccount.PasswordCredential, password string) error {
	_, span := gstotel.StartSpan(ctx, gstotel.OperationSpanName("iam", "VerifyPassword"))
	defer span.End()

	if credential == nil {
		err := errors.New("password credential is required")
		gstotel.RecordError(span, err)
		return err
	}

	err := bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(password))
	gstotel.AddSpanTags(span, map[string]any{"iam.password.match": err == nil})
	if err != nil && !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		gstotel.RecordError(span, err)
	}
	return err
}

// ApplyPasswordCredentialUpdate replaces the credential hash and password-change state.
func ApplyPasswordCredentialUpdate(ctx context.Context, credential *modeliamaccount.PasswordCredential, newPassword string, mustChangePassword bool) error {
	if credential == nil {
		return service.NewError(http.StatusInternalServerError, "password credential is required")
	}

	passwordHash, err := hashAccountPassword(ctx, newPassword)
	if err != nil {
		return err
	}
	now := time.Now()
	credential.PasswordHash = passwordHash
	credential.MustChangePassword = mustChangePassword
	credential.PasswordChangedAt = &now
	return nil
}

// hashAccountPassword hashes a plaintext password with bcrypt. Like the bcrypt
// comparison in VerifyPasswordCredential, hashing is deliberately slow (tens of
// milliseconds) to resist brute-force attacks, so it runs inside an
// iam.HashPassword span to keep that cost visible in request traces.
func hashAccountPassword(ctx context.Context, password string) (string, error) {
	if err := validateNewAccountPassword(password); err != nil {
		return "", err
	}

	_, span := gstotel.StartSpan(ctx, gstotel.OperationSpanName("iam", "HashPassword"))
	defer span.End()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		serviceErr := service.NewErrorWithCause(http.StatusInternalServerError, "failed to hash password", err)
		gstotel.RecordError(span, serviceErr)
		return "", serviceErr
	}
	return string(hashedPassword), nil
}
