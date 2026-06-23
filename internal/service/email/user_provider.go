package serviceemail

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
)

var (
	// ErrUserProviderNotConfigured is returned until the host application
	// installs a real UserProvider for email account operations.
	ErrUserProviderNotConfigured = errors.New("email user provider is not configured")

	// ErrUserProviderInvalidUser means a provider returned a user snapshot that
	// does not satisfy the email module identity contract.
	ErrUserProviderInvalidUser = errors.New("email user provider returned invalid user")

	// ErrUserNotFound lets providers report missing accounts without leaking the
	// concrete user-store implementation into email services.
	ErrUserNotFound = errors.New("email user not found")

	// ErrUserAuthenticationFailed hides user-not-found, inactive-account, and
	// invalid-password details behind one safe verification failure signal.
	ErrUserAuthenticationFailed = errors.New("email user authentication failed")
)

var (
	userProviderMu sync.RWMutex
	userProvider   UserProvider = missingUserProvider{}
)

var _ UserProvider = (*missingUserProvider)(nil)

// UserProvider connects the email module to the host application's account
// system. Implementations own user lookup, password verification, password
// hashing, account updates, and session invalidation for their user store.
//
// The email service intentionally depends only on this provider and UserSnapshot
// so copied email module code does not import a concrete IAM user model.
type UserProvider interface {
	// FindByEmail resolves the account currently bound to email.
	FindByEmail(ctx *types.ServiceContext, email string) (*UserSnapshot, error)

	// GetByID loads a stable account snapshot by user ID.
	GetByID(ctx *types.ServiceContext, userID string) (*UserSnapshot, error)

	// VerifyPassword validates the current password for the user.
	VerifyPassword(ctx *types.ServiceContext, userID, password string) error

	// ResetPassword persists a new password according to the host password policy.
	ResetPassword(ctx *types.ServiceContext, userID, newPassword string) error

	// MarkEmailVerified persists the email verification state for the user.
	MarkEmailVerified(ctx *types.ServiceContext, userID string, verifiedAt time.Time) error

	// ChangeEmail persists a confirmed email change for the user.
	ChangeEmail(ctx *types.ServiceContext, userID, newEmail string, changedAt time.Time) error

	// InvalidateSessions revokes cached sessions for a user after password reset.
	InvalidateSessions(userID string)
}

// UserSnapshot is the minimal account state required by email flows. ID must be
// the same stable user identifier stored in email flow state. Active must be true
// only when the host account is allowed to start or complete email flows.
type UserSnapshot struct {
	ID            string
	Email         string
	Active        bool
	EmailVerified bool
}

// SetUserProvider installs the host application's user provider. Call it during
// application/module initialization before serving email routes. Passing nil
// restores the safe default that fails user-backed email flows with
// ErrUserProviderNotConfigured.
func SetUserProvider(provider UserProvider) {
	userProviderMu.Lock()
	defer userProviderMu.Unlock()

	if provider == nil {
		userProvider = missingUserProvider{}
		return
	}
	userProvider = provider
}

func currentUserProvider() UserProvider {
	userProviderMu.RLock()
	defer userProviderMu.RUnlock()

	return userProvider
}

func validUserSnapshot(user *UserSnapshot, expectedUserID string) error {
	if user == nil {
		return errors.Wrap(ErrUserProviderInvalidUser, "nil user snapshot")
	}
	if strings.TrimSpace(user.ID) == "" {
		return errors.Wrap(ErrUserProviderInvalidUser, "empty user id")
	}
	if expectedUserID != "" && user.ID != expectedUserID {
		return errors.Wrap(ErrUserProviderInvalidUser, "user id mismatch")
	}
	return nil
}

func newUserProviderNotConfiguredServiceError(err error) *types.ServiceError {
	return types.NewServiceErrorWithCause(http.StatusInternalServerError, "Email user provider is not configured", err)
}

func newUserProviderInvalidUserServiceError(err error) *types.ServiceError {
	return types.NewServiceErrorWithCause(http.StatusInternalServerError, "Email user provider returned invalid user", err)
}

// missingUserProvider is the safe default used until the host application
// installs a real UserProvider. It keeps copied email code buildable and makes
// user-backed flows fail with a clear configuration error.
type missingUserProvider struct{}

func (missingUserProvider) FindByEmail(*types.ServiceContext, string) (*UserSnapshot, error) {
	return nil, ErrUserProviderNotConfigured
}

func (missingUserProvider) GetByID(*types.ServiceContext, string) (*UserSnapshot, error) {
	return nil, ErrUserProviderNotConfigured
}

func (missingUserProvider) VerifyPassword(*types.ServiceContext, string, string) error {
	return ErrUserProviderNotConfigured
}

func (missingUserProvider) ResetPassword(*types.ServiceContext, string, string) error {
	return ErrUserProviderNotConfigured
}

func (missingUserProvider) MarkEmailVerified(*types.ServiceContext, string, time.Time) error {
	return ErrUserProviderNotConfigured
}

func (missingUserProvider) ChangeEmail(*types.ServiceContext, string, string, time.Time) error {
	return ErrUserProviderNotConfigured
}

func (missingUserProvider) InvalidateSessions(string) {}
