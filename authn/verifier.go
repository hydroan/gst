// Package authn exposes the authentication extension points that optional
// modules install into the mandatory IAM login flow.
//
// Two hook kinds exist with deliberately different semantics:
//
//   - The login second-factor verifier is a gate: at most one verifier is
//     installed, and a non-nil error from it rejects the login.
//   - Login observers are bystanders: any number can be added, they hear
//     login lifecycle events after the outcome is settled, and they can
//     never block or fail the login itself.
//
// Optional modules install their hooks from package init so the add path
// (framework module registration) and the copy path (project-owned copied
// source) share one mechanism with no manual wiring steps.
package authn

import (
	"sync"

	"github.com/hydroan/gst/types"
)

// LoginSecondFactor carries the second-factor proof fields submitted with an
// IAM login request.
type LoginSecondFactor struct {
	TOTPCode   string
	BackupCode string
}

// LoginSecondFactorVerifier decides the second factor of one IAM login
// attempt after the first factor already passed. Returning nil lets the login
// proceed. A non-nil error rejects the login and reaches the login caller
// unchanged, so the verifier owns the client-facing error shape.
type LoginSecondFactorVerifier func(ctx *types.ServiceContext, userID string, factor LoginSecondFactor) error

var (
	verifierMu                sync.Mutex
	loginSecondFactorVerifier LoginSecondFactorVerifier
)

// SetLoginSecondFactorVerifier installs the login second-factor gate.
//
// The gate is unique: installing a verifier while another one is installed
// panics, because two competing gates are a wiring error that must surface at
// startup instead of resolving silently by call order. Passing nil uninstalls
// the current verifier; tests use that to restore the default.
func SetLoginSecondFactorVerifier(verifier LoginSecondFactorVerifier) {
	verifierMu.Lock()
	defer verifierMu.Unlock()

	if verifier != nil && loginSecondFactorVerifier != nil {
		panic("authn: a login second-factor verifier is already installed")
	}
	loginSecondFactorVerifier = verifier
}

// VerifyLoginSecondFactor runs the installed second-factor gate for one login
// attempt. Without an installed verifier there is no second-factor
// requirement and the attempt passes.
func VerifyLoginSecondFactor(ctx *types.ServiceContext, userID string, factor LoginSecondFactor) error {
	verifierMu.Lock()
	verifier := loginSecondFactorVerifier
	verifierMu.Unlock()

	if verifier == nil {
		return nil
	}
	return verifier(ctx, userID, factor)
}
