package serviceiamaccount

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

const (
	defaultLoginFailureLimit  = 5
	defaultLoginFailureWindow = 15 * time.Minute

	loginFailureLimitEnv  = "IAM_LOGIN_FAILURE_LIMIT"
	loginFailureWindowEnv = "IAM_LOGIN_FAILURE_WINDOW"
)

// ensureLoginNotLockedOut refuses an account that has failed too many times
// recently.
//
// The refusal is worded exactly like a wrong password, and deliberately so.
// Saying "locked" instead would answer a question the caller has no right to
// ask: an attacker can fail five times against any username and read the
// difference between the two replies as whether that account exists. The cost
// is that a locked-out user is not told why, which is the side to be wrong on
// while there is no per-address throttle in front of this.
func ensureLoginNotLockedOut(ctx *types.ServiceContext, username string) error {
	if serviceiamsession.Store.LoginFailures(ctx, username) < loginFailureLimit() {
		return nil
	}
	return service.NewError(http.StatusUnauthorized, "invalid username or password")
}

// recordLoginFailure counts one failed credential attempt against an account.
//
// Only attempts against an account that exists are counted. Counting the rest
// would let anyone fill Redis with a key per username they invent, and the
// account they are guessing at is the only one a lockout protects anyway.
func recordLoginFailure(ctx *types.ServiceContext, username string) {
	serviceiamsession.Store.RecordLoginFailure(ctx, username, loginFailureWindow())
}

// clearLoginFailures forgets an account's failed attempts after it proves the
// password, so a user who eventually gets it right starts from zero rather than
// one attempt short of a lockout.
func clearLoginFailures(ctx *types.ServiceContext, username string) {
	serviceiamsession.Store.ClearLoginFailures(ctx, username)
}

// loginFailureLimit is how many consecutive failures lock an account, read from
// IAM_LOGIN_FAILURE_LIMIT and defaulting to 5.
func loginFailureLimit() int64 {
	raw := strings.TrimSpace(os.Getenv(loginFailureLimitEnv))
	if raw == "" {
		return defaultLoginFailureLimit
	}

	limit, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		panic(errors.Wrapf(err, "invalid %s", loginFailureLimitEnv))
	}
	if limit <= 0 {
		panic(errors.Errorf("%s must be greater than 0", loginFailureLimitEnv))
	}
	return limit
}

// loginFailureWindow is how long a lockout lasts, read from
// IAM_LOGIN_FAILURE_WINDOW and defaulting to 15 minutes.
//
// The window is what ends a lockout; nothing else clears it, so a value long
// enough to be an outage for a user who mistypes their password is the failure
// mode to watch for when changing it.
func loginFailureWindow() time.Duration {
	raw := strings.TrimSpace(os.Getenv(loginFailureWindowEnv))
	if raw == "" {
		return defaultLoginFailureWindow
	}

	window, err := time.ParseDuration(raw)
	if err != nil {
		panic(errors.Wrapf(err, "invalid %s", loginFailureWindowEnv))
	}
	if window <= 0 {
		panic(errors.Errorf("%s must be greater than 0", loginFailureWindowEnv))
	}
	return window
}
