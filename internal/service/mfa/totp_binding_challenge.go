package servicemfa

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

const (
	totpBindChallengeTTL     = 10 * time.Minute
	totpBindChallengeKeyBase = "mfa:totp:bind"
)

// totpBindChallenge stores one pending TOTP binding attempt in cache.
//
// The value keeps the generated TOTP secret on the server and binds it to the
// user and session that started the flow. Confirm requests must present the
// challenge ID and a valid TOTP code; they never provide the secret directly.
type totpBindChallenge struct {
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id"`
	Username  string    `json:"username"`
	Secret    string    `json:"secret"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

var (
	errTOTPBindChallengeNotFound = errors.New("totp bind challenge not found")
	errTOTPBindChallengeExpired  = errors.New("totp bind challenge expired")
	errTOTPBindChallengeInvalid  = errors.New("totp bind challenge invalid")

	totpBindChallengeCache = func() types.Cache[totpBindChallenge] {
		return redis.Cache[totpBindChallenge]()
	}
	totpBindChallengeNow = func() time.Time { return time.Now().UTC() }
)

// currentTOTPBindSessionID returns the session that owns the current binding flow.
//
// Service context is the preferred source. The cookie fallback keeps the helper
// usable in routes where middleware has authenticated the request but the
// session ID has not been copied into the context field.
func currentTOTPBindSessionID(ctx *types.ServiceContext) (string, error) {
	if ctx == nil {
		return "", service.NewError(http.StatusUnauthorized, "authentication required")
	}
	if strings.TrimSpace(ctx.SessionID) != "" {
		return strings.TrimSpace(ctx.SessionID), nil
	}
	sessionID, err := ctx.Cookie("session_id")
	if err != nil {
		return "", service.NewError(http.StatusUnauthorized, "authentication required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", service.NewError(http.StatusUnauthorized, "authentication required")
	}
	return sessionID, nil
}

// issueTOTPBindChallenge creates a cache-backed challenge for a pending TOTP binding flow.
//
// It validates the user, session, username, and generated secret, records
// issue/expiry timestamps, and stores the challenge with the same TTL in Redis.
// The returned challenge ID is the only value clients submit during confirm.
func issueTOTPBindChallenge(ctx context.Context, challenge totpBindChallenge) (string, totpBindChallenge, error) {
	if strings.TrimSpace(challenge.UserID) == "" ||
		strings.TrimSpace(challenge.SessionID) == "" ||
		strings.TrimSpace(challenge.Username) == "" ||
		strings.TrimSpace(challenge.Secret) == "" {
		return "", totpBindChallenge{}, errTOTPBindChallengeInvalid
	}

	challengeID := util.UUID()
	now := totpBindChallengeNow()
	challenge.UserID = strings.TrimSpace(challenge.UserID)
	challenge.SessionID = strings.TrimSpace(challenge.SessionID)
	challenge.Username = strings.TrimSpace(challenge.Username)
	challenge.Secret = strings.TrimSpace(challenge.Secret)
	challenge.IssuedAt = now
	challenge.ExpiresAt = now.Add(totpBindChallengeTTL)

	if err := totpBindChallengeCache().WithContext(normalizeTOTPBindContext(ctx)).
		Set(totpBindChallengeKey(challengeID), challenge, totpBindChallengeTTL); err != nil {
		return "", totpBindChallenge{}, errors.Wrap(err, "store TOTP binding challenge")
	}

	return challengeID, challenge, nil
}

// loadTOTPBindChallenge returns a pending challenge after validating its required fields and expiry.
//
// Missing, malformed, or expired challenges are mapped to local sentinel errors
// so public services can expose the same generic "invalid or expired" response.
// Expired values are best-effort deleted from cache after detection.
func loadTOTPBindChallenge(ctx context.Context, challengeID string) (totpBindChallenge, error) {
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return totpBindChallenge{}, errTOTPBindChallengeNotFound
	}

	challenge, err := totpBindChallengeCache().WithContext(normalizeTOTPBindContext(ctx)).
		Get(totpBindChallengeKey(challengeID))
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return totpBindChallenge{}, errTOTPBindChallengeNotFound
		}
		return totpBindChallenge{}, errors.Wrap(err, "load TOTP binding challenge")
	}
	if strings.TrimSpace(challenge.UserID) == "" ||
		strings.TrimSpace(challenge.SessionID) == "" ||
		strings.TrimSpace(challenge.Secret) == "" {
		return totpBindChallenge{}, errTOTPBindChallengeInvalid
	}
	if !challenge.ExpiresAt.IsZero() && !challenge.ExpiresAt.After(totpBindChallengeNow()) {
		_ = totpBindChallengeCache().WithContext(normalizeTOTPBindContext(ctx)).
			Delete(totpBindChallengeKey(challengeID))
		return totpBindChallenge{}, errTOTPBindChallengeExpired
	}

	return challenge, nil
}

// consumeTOTPBindChallenge deletes a confirmed challenge so it cannot be reused.
//
// Confirm calls this only after the active device has been created, preserving
// the challenge when the user submits a wrong TOTP code or storage fails.
func consumeTOTPBindChallenge(ctx context.Context, challengeID string) error {
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return errTOTPBindChallengeNotFound
	}
	if err := totpBindChallengeCache().WithContext(normalizeTOTPBindContext(ctx)).
		Delete(totpBindChallengeKey(challengeID)); err != nil {
		return errors.Wrap(err, "consume TOTP binding challenge")
	}
	return nil
}

// totpBindChallengeKey builds the cache key for a pending TOTP binding challenge.
func totpBindChallengeKey(challengeID string) string {
	return strings.Join([]string{totpBindChallengeKeyBase, strings.TrimSpace(challengeID)}, ":")
}

// normalizeTOTPBindContext provides a non-nil context for cache operations.
func normalizeTOTPBindContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
