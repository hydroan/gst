package servicemfa

import (
	"context"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	gstredis "github.com/hydroan/gst/redis"
)

// totpCodeReplayTTL outlives the ±1-period validation window (about 90 seconds
// at the default 30-second period with Skew 1), so a marked code stays rejected
// until it can no longer validate anyway.
const totpCodeReplayTTL = 2 * time.Minute

// totpCodeReplayKeyPrefix opens the one-time-use marker key of a validated
// code, followed by the user and the code.
const totpCodeReplayKeyPrefix = "mfa:totp:used"

var errTOTPCodeReplayed = errors.New("TOTP code already used")

// markTOTPCodeUsed consumes one successfully validated TOTP code.
//
// RFC 6238 section 5.2 requires rejecting a second submission of the same OTP.
// The marker is per user and code so one intercepted code cannot be replayed
// across login, unbind, and confirm within its validation window. The
// marker store is authoritative: when it is unreachable the code is rejected
// instead of silently accepted (fail closed).
func markTOTPCodeUsed(ctx context.Context, userID, code string) error {
	key := strings.Join([]string{totpCodeReplayKeyPrefix, strings.TrimSpace(userID), strings.TrimSpace(code)}, ":")
	ok, err := gstredis.SetNX(ctx, key, "1", totpCodeReplayTTL)
	if err != nil {
		return errors.Wrap(err, "mark TOTP code used")
	}
	if !ok {
		return errTOTPCodeReplayed
	}
	return nil
}
