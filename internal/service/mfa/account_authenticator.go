package servicemfa

import (
	"net/http"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var (
	// ErrAccountAuthenticatorNotConfigured is returned by password-based MFA flows
	// until the host application installs a real AccountAuthenticator.
	ErrAccountAuthenticatorNotConfigured = errors.New("mfa account authenticator is not configured")

	// ErrAccountAuthenticatorInvalidAccount means an authenticator reported success
	// but returned an account that does not satisfy MFA's identity contract.
	ErrAccountAuthenticatorInvalidAccount = errors.New("mfa account authenticator returned invalid account")

	// ErrAccountAuthenticationFailed lets authenticators hide account-not-found,
	// invalid-password, locked, and inactive-account details behind one safe
	// authentication failure signal.
	ErrAccountAuthenticationFailed = errors.New("mfa account authentication failed")
)

var (
	accountAuthenticatorMu sync.RWMutex
	accountAuthenticator   AccountAuthenticator = missingAccountAuthenticator{}
)

var _ AccountAuthenticator = (*missingAccountAuthenticator)(nil)

// AccountAuthenticator connects MFA password-based flows to the host application's
// primary account system. Implementations must query the application's account store,
// validate the supplied password with the application's password policy, and
// return the authenticated account's stable ID.
//
// MFA intentionally does not own account lookup or password verification. Projects
// copied with `gg module copy mfa` should install their implementation from a
// project-owned file outside service/mfa, for example module/mfa_account_authenticator.go.
type AccountAuthenticator interface {
	// AuthenticateByUsername is used by TOTP check before login completes.
	AuthenticateByUsername(ctx *types.ServiceContext, username, password string) (*AuthenticatedAccount, error)

	// AuthenticateByAccountID is used by password-based fresh authentication, such
	// as TOTP unbind.
	AuthenticateByAccountID(ctx *types.ServiceContext, accountID, password string) (*AuthenticatedAccount, error)
}

// AuthenticatedAccount is the minimal identity MFA needs after primary
// authentication succeeds. ID must be the same stable account identifier stored in
// TOTPDevice.UserID; Username should be set when the authenticator has it.
type AuthenticatedAccount struct {
	ID       string
	Username string
}

// SetAccountAuthenticator installs the host application's account authenticator. Call
// it during application/module initialization before serving requests. Passing
// nil restores the safe default that fails password-based MFA flows with
// ErrAccountAuthenticatorNotConfigured.
func SetAccountAuthenticator(auth AccountAuthenticator) {
	accountAuthenticatorMu.Lock()
	defer accountAuthenticatorMu.Unlock()

	if auth == nil {
		accountAuthenticator = missingAccountAuthenticator{}
		return
	}
	accountAuthenticator = auth
}

func currentAccountAuthenticator() AccountAuthenticator {
	accountAuthenticatorMu.RLock()
	defer accountAuthenticatorMu.RUnlock()

	return accountAuthenticator
}

func validateAuthenticatedAccount(account *AuthenticatedAccount, expectedAccountID string) error {
	if account == nil {
		return errors.Wrap(ErrAccountAuthenticatorInvalidAccount, "nil authenticated account")
	}
	if strings.TrimSpace(account.ID) == "" {
		return errors.Wrap(ErrAccountAuthenticatorInvalidAccount, "empty authenticated account id")
	}
	if expectedAccountID != "" && account.ID != expectedAccountID {
		return errors.Wrap(ErrAccountAuthenticatorInvalidAccount, "authenticated account id mismatch")
	}
	return nil
}

func newAccountAuthenticatorNotConfiguredServiceError(err error) *service.Error {
	return service.NewErrorWithCause(http.StatusInternalServerError, "MFA account authenticator is not configured", err)
}

func newAccountAuthenticatorInvalidAccountServiceError(err error) *service.Error {
	return service.NewErrorWithCause(http.StatusInternalServerError, "MFA account authenticator returned invalid account", err)
}

// missingAccountAuthenticator is the safe default used until the host application
// installs a real AccountAuthenticator. It keeps copied MFA code buildable and
// makes password-based MFA flows fail with a clear configuration error instead
// of panicking or importing a framework user model.
type missingAccountAuthenticator struct{}

func (missingAccountAuthenticator) AuthenticateByUsername(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
	return nil, ErrAccountAuthenticatorNotConfigured
}

func (missingAccountAuthenticator) AuthenticateByAccountID(*types.ServiceContext, string, string) (*AuthenticatedAccount, error) {
	return nil, ErrAccountAuthenticatorNotConfigured
}
