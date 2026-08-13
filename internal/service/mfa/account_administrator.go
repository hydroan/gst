package servicemfa

import (
	"net/http"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ErrAccountAdministratorNotConfigured is returned by administrative MFA flows
// until the host application installs a real AccountAdministrator.
var ErrAccountAdministratorNotConfigured = errors.New("mfa account administrator is not configured")

// AccountAdministrator authorizes administrative MFA operations on a target
// account. Implementations decide who may inspect or reset another account's
// enrollment; the built-in module adapter delegates to the framework IAM
// tenant-admin rules.
//
// MFA intentionally does not own that authority model. Projects copied with
// `gg module copy mfa` install their implementation from a project-owned file
// outside service/mfa; until one is installed every administrative call is
// denied.
type AccountAdministrator interface {
	// EnsureCanAdminister rejects the request unless the current actor may
	// administer the target account's MFA enrollment.
	EnsureCanAdminister(ctx *types.ServiceContext, targetUserID string) error
}

var (
	accountAdministratorMu sync.RWMutex
	accountAdministrator   AccountAdministrator = missingAccountAdministrator{}
)

// SetAccountAdministrator installs the host application's administrative
// authorizer. Call it during application/module initialization before serving
// requests. Passing nil restores the safe default that denies every
// administrative MFA call with ErrAccountAdministratorNotConfigured.
func SetAccountAdministrator(admin AccountAdministrator) {
	accountAdministratorMu.Lock()
	defer accountAdministratorMu.Unlock()

	if admin == nil {
		accountAdministrator = missingAccountAdministrator{}
		return
	}
	accountAdministrator = admin
}

func currentAccountAdministrator() AccountAdministrator {
	accountAdministratorMu.RLock()
	defer accountAdministratorMu.RUnlock()

	return accountAdministrator
}

// ensureCanAdministerMFA runs the installed administrative authorizer and
// shapes its outcome as a service error: adapters already answer with one and
// are passed through untouched, while anything else is reported as 500.
func ensureCanAdministerMFA(ctx *types.ServiceContext, targetUserID string) *service.Error {
	err := currentAccountAdministrator().EnsureCanAdminister(ctx, targetUserID)
	if err == nil {
		return nil
	}
	var svcErr *service.Error
	if errors.As(err, &svcErr) {
		return svcErr
	}
	return service.NewErrorWithCause(http.StatusInternalServerError, "administrative authorization failed", err)
}

// missingAccountAdministrator is the deny-by-default used until the host
// application installs a real AccountAdministrator. It keeps copied MFA code
// buildable while making the missing wiring loud instead of silently granting
// administrative access.
type missingAccountAdministrator struct{}

func (missingAccountAdministrator) EnsureCanAdminister(*types.ServiceContext, string) error {
	return service.NewErrorWithCause(http.StatusInternalServerError,
		"MFA account administrator is not configured", ErrAccountAdministratorNotConfigured)
}
