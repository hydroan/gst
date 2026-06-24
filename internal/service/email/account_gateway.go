package serviceemail

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var (
	// ErrAccountGatewayNotConfigured is returned until the host application
	// installs a real AccountGateway for email account operations.
	ErrAccountGatewayNotConfigured = errors.New("email account gateway is not configured")

	// ErrAccountGatewayInvalidAccount means a gateway returned an account snapshot that
	// does not satisfy the email module identity contract.
	ErrAccountGatewayInvalidAccount = errors.New("email account gateway returned invalid account")

	// ErrAccountNotFound lets gateways report missing accounts without leaking the
	// concrete account store implementation into email services.
	ErrAccountNotFound = errors.New("email account not found")

	// ErrAccountAuthenticationFailed hides account-not-found, inactive-account, and
	// invalid-password details behind one safe verification failure signal.
	ErrAccountAuthenticationFailed = errors.New("email account authentication failed")
)

var (
	accountGatewayMu sync.RWMutex
	accountGateway   AccountGateway = missingAccountGateway{}
)

var _ AccountGateway = (*missingAccountGateway)(nil)

// AccountGateway connects the email module to the host application's account
// system. Implementations own account lookup, password verification, password
// hashing, account updates, and session invalidation for their account store.
//
// The email service intentionally depends only on this gateway and AccountSnapshot
// so copied email module code does not import a concrete IAM user model.
type AccountGateway interface {
	// FindByEmail resolves the account currently bound to email.
	FindByEmail(ctx *types.ServiceContext, email string) (*AccountSnapshot, error)

	// GetByID loads a stable account snapshot by account ID.
	GetByID(ctx *types.ServiceContext, userID string) (*AccountSnapshot, error)

	// VerifyPassword validates the current password for the account.
	VerifyPassword(ctx *types.ServiceContext, userID, password string) error

	// UpdatePassword persists a new password according to the host password policy.
	UpdatePassword(ctx *types.ServiceContext, userID, newPassword string) error

	// MarkEmailVerified persists the email verification state for the account.
	MarkEmailVerified(ctx *types.ServiceContext, userID string, verifiedAt time.Time) error

	// ApplyEmailChange persists a confirmed email change for the account.
	ApplyEmailChange(ctx *types.ServiceContext, userID, newEmail string, changedAt time.Time) error

	// InvalidateSessions revokes cached sessions for an account after password reset.
	InvalidateSessions(userID string)
}

// AccountSnapshot is the minimal account state required by email flows. ID must be
// the same stable account identifier stored in email flow state. Active must be true
// only when the host account is allowed to start or complete email flows.
type AccountSnapshot struct {
	ID            string
	Email         string
	Active        bool
	EmailVerified bool
}

// SetAccountGateway installs the host application's account gateway. Call it during
// application/module initialization before serving email routes. Passing nil
// restores the safe default that fails account-backed email flows with
// ErrAccountGatewayNotConfigured.
func SetAccountGateway(gateway AccountGateway) {
	accountGatewayMu.Lock()
	defer accountGatewayMu.Unlock()

	if gateway == nil {
		accountGateway = missingAccountGateway{}
		return
	}
	accountGateway = gateway
}

func currentAccountGateway() AccountGateway {
	accountGatewayMu.RLock()
	defer accountGatewayMu.RUnlock()

	return accountGateway
}

func validAccountSnapshot(account *AccountSnapshot, expectedUserID string) error {
	if account == nil {
		return errors.Wrap(ErrAccountGatewayInvalidAccount, "nil account snapshot")
	}
	if strings.TrimSpace(account.ID) == "" {
		return errors.Wrap(ErrAccountGatewayInvalidAccount, "empty account id")
	}
	if expectedUserID != "" && account.ID != expectedUserID {
		return errors.Wrap(ErrAccountGatewayInvalidAccount, "account id mismatch")
	}
	return nil
}

func newAccountGatewayNotConfiguredServiceError(err error) *service.Error {
	return service.NewErrorWithCause(http.StatusInternalServerError, "Email account gateway is not configured", err)
}

func newAccountGatewayInvalidAccountServiceError(err error) *service.Error {
	return service.NewErrorWithCause(http.StatusInternalServerError, "Email account gateway returned invalid account", err)
}

// missingAccountGateway is the safe default used until the host application
// installs a real AccountGateway. It keeps copied email code buildable and makes
// account-backed flows fail with a clear configuration error.
type missingAccountGateway struct{}

func (missingAccountGateway) FindByEmail(*types.ServiceContext, string) (*AccountSnapshot, error) {
	return nil, ErrAccountGatewayNotConfigured
}

func (missingAccountGateway) GetByID(*types.ServiceContext, string) (*AccountSnapshot, error) {
	return nil, ErrAccountGatewayNotConfigured
}

func (missingAccountGateway) VerifyPassword(*types.ServiceContext, string, string) error {
	return ErrAccountGatewayNotConfigured
}

func (missingAccountGateway) UpdatePassword(*types.ServiceContext, string, string) error {
	return ErrAccountGatewayNotConfigured
}

func (missingAccountGateway) MarkEmailVerified(*types.ServiceContext, string, time.Time) error {
	return ErrAccountGatewayNotConfigured
}

func (missingAccountGateway) ApplyEmailChange(*types.ServiceContext, string, string, time.Time) error {
	return ErrAccountGatewayNotConfigured
}

func (missingAccountGateway) InvalidateSessions(string) {}
