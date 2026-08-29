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
// Optional modules install their hooks explicitly: module registration calls
// them on the add path, project-owned assembly code calls them on the copy
// path. Nothing installs itself from package init, because a copied service
// package is linked only when generated code happens to import it, and route
// configuration can silently take that import away.
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

// MsgSecondFactorRequired is the challenge a verifier reports with 401 when the
// account needs a second factor it has not submitted. Login UIs match it to
// decide whether to prompt for a code, so the wording is a client contract and
// must stay stable.
//
// It lives here rather than with any one verifier because the challenge belongs
// to the login protocol, not to a kind of factor: a UI must recognize it
// whether the installed verifier checks TOTP, SMS, or anything else.
// Proof-specific rejections stay the verifier's own wording.
const MsgSecondFactorRequired = "second factor required"

// LoginSecondFactorVerifier decides the second factor of one IAM login
// attempt after the first factor already passed. Returning nil lets the login
// proceed. A non-nil error rejects the login and reaches the login caller
// unchanged, so the verifier owns the client-facing error shape. For the one
// challenge every login UI must recognize, report MsgSecondFactorRequired with
// 401 instead of wording it again.
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
