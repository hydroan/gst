package servicemfa

import (
	"context"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/redis"
)

// A user may submit this many second-factor proofs to one purpose within a
// window before further proofs are refused. The window opens with the first
// counted attempt and is never extended, so a lockout always ends a fixed time
// after it began and a stream of guesses cannot hold an account locked.
const (
	totpVerificationAttemptLimit  = 5
	totpVerificationAttemptWindow = 15 * time.Minute
)

// totpVerificationPurpose names an endpoint family that keeps its own attempt
// budget. Login and unbind are counted apart, so spending one budget neither
// locks nor resets the other.
type totpVerificationPurpose string

const (
	totpVerificationLogin  totpVerificationPurpose = "login"
	totpVerificationUnbind totpVerificationPurpose = "unbind"
)

var errTOTPVerificationLocked = errors.New("too many failed TOTP verification attempts")

// reserveTOTPVerificationAttempt spends one attempt from the user's budget for
// purpose before a proof is checked.
//
// The attempt is paid up front instead of being recorded after a failure: a
// check-then-record counter lets concurrent guesses all pass the check before
// any of them is recorded, while an atomic reservation caps them at the limit
// however they interleave. A verified proof resets the budget through
// clearTOTPVerificationFailures. The counter lives in Redis so every replica
// draws on the same budget, and it is authoritative: when it cannot be reached
// the attempt is refused (fail closed) and no proof may be checked, so an
// outage neither opens unlimited guessing nor spends a recovery code.
func reserveTOTPVerificationAttempt(ctx context.Context, purpose totpVerificationPurpose, userID string) error {
	count, err := redis.IncrFixedWindow(ctx, totpVerificationFailureKey(purpose, userID), totpVerificationAttemptWindow)
	if err != nil {
		return errors.Wrap(err, "reserve TOTP verification attempt")
	}
	if count > totpVerificationAttemptLimit {
		return errTOTPVerificationLocked
	}
	return nil
}

// clearTOTPVerificationFailures forgets the user's counted attempts for each
// purpose.
//
// Clearing is best effort: a counter left behind only keeps the user nearer to
// a lockout that still ends with its window, which beats failing an operation
// that already succeeded.
func clearTOTPVerificationFailures(ctx context.Context, userID string, purposes ...totpVerificationPurpose) {
	keys := make([]string, 0, len(purposes))
	for _, purpose := range purposes {
		keys = append(keys, totpVerificationFailureKey(purpose, userID))
	}
	_ = redis.Del(ctx, keys...)
}

// totpVerificationFailureKey builds the attempt counter key for one user and purpose.
func totpVerificationFailureKey(purpose totpVerificationPurpose, userID string) string {
	return strings.Join([]string{"mfa:totp:failure", string(purpose), strings.TrimSpace(userID)}, ":")
}
