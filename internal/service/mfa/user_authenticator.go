package servicemfa

import (
	"net/http"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
)

var (
	// ErrUserAuthenticatorNotConfigured is returned by password-based MFA flows
	// until the host application installs a real UserAuthenticator.
	ErrUserAuthenticatorNotConfigured = errors.New("mfa user authenticator is not configured")

	// ErrUserAuthenticatorInvalidUser means an authenticator reported success but
	// returned a user that does not satisfy MFA's identity contract.
	ErrUserAuthenticatorInvalidUser = errors.New("mfa user authenticator returned invalid user")

	// ErrUserAuthenticationFailed lets authenticators hide user-not-found,
	// invalid-password, locked, and inactive-account details behind one safe
	// authentication failure signal.
	ErrUserAuthenticationFailed = errors.New("mfa user authentication failed")
)

var (
	userAuthenticatorMu sync.RWMutex
	userAuthenticator   UserAuthenticator = missingUserAuthenticator{}
)

var _ UserAuthenticator = (*missingUserAuthenticator)(nil)

// UserAuthenticator connects MFA password-based flows to the host application's
// primary user system. Implementations must query the application's user store,
// validate the supplied password with the application's password policy, and
// return the authenticated user's stable ID.
//
// MFA intentionally does not own user lookup or password verification. Projects
// copied with `gg module copy mfa` should install their implementation from a
// project-owned file outside service/mfa, for example module/mfa_user_authenticator.go.
type UserAuthenticator interface {
	// AuthenticateByUsername is used by TOTP check before login completes.
	AuthenticateByUsername(ctx *types.ServiceContext, username, password string) (*AuthenticatedUser, error)

	// AuthenticateByUserID is used by password-based fresh authentication, such
	// as TOTP unbind.
	AuthenticateByUserID(ctx *types.ServiceContext, userID, password string) (*AuthenticatedUser, error)
}

// AuthenticatedUser is the minimal identity MFA needs after primary
// authentication succeeds. ID must be the same stable user identifier stored in
// TOTPDevice.UserID; Username should be set when the authenticator has it.
type AuthenticatedUser struct {
	ID       string
	Username string
}

// SetUserAuthenticator installs the host application's user authenticator. Call
// it during application/module initialization before serving requests. Passing
// nil restores the safe default that fails password-based MFA flows with
// ErrUserAuthenticatorNotConfigured.
func SetUserAuthenticator(auth UserAuthenticator) {
	userAuthenticatorMu.Lock()
	defer userAuthenticatorMu.Unlock()

	if auth == nil {
		userAuthenticator = missingUserAuthenticator{}
		return
	}
	userAuthenticator = auth
}

func currentUserAuthenticator() UserAuthenticator {
	userAuthenticatorMu.RLock()
	defer userAuthenticatorMu.RUnlock()

	return userAuthenticator
}

func validateAuthenticatedUser(user *AuthenticatedUser, expectedUserID string) error {
	if user == nil {
		return errors.Wrap(ErrUserAuthenticatorInvalidUser, "nil authenticated user")
	}
	if strings.TrimSpace(user.ID) == "" {
		return errors.Wrap(ErrUserAuthenticatorInvalidUser, "empty authenticated user id")
	}
	if expectedUserID != "" && user.ID != expectedUserID {
		return errors.Wrap(ErrUserAuthenticatorInvalidUser, "authenticated user id mismatch")
	}
	return nil
}

func newUserAuthenticatorNotConfiguredServiceError(err error) *types.ServiceError {
	return types.NewServiceErrorWithCause(http.StatusInternalServerError, "MFA user authenticator is not configured", err)
}

func newUserAuthenticatorInvalidUserServiceError(err error) *types.ServiceError {
	return types.NewServiceErrorWithCause(http.StatusInternalServerError, "MFA user authenticator returned invalid user", err)
}

// missingUserAuthenticator is the safe default used until the host application
// installs a real UserAuthenticator. It keeps copied MFA code buildable and
// makes password-based MFA flows fail with a clear configuration error instead
// of panicking or importing a framework user model.
type missingUserAuthenticator struct{}

func (missingUserAuthenticator) AuthenticateByUsername(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
	return nil, ErrUserAuthenticatorNotConfigured
}

func (missingUserAuthenticator) AuthenticateByUserID(*types.ServiceContext, string, string) (*AuthenticatedUser, error) {
	return nil, ErrUserAuthenticatorNotConfigured
}
