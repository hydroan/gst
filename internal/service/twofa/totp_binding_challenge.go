package servicetwofa

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

const (
	totpBindChallengeTTL     = 10 * time.Minute
	totpBindChallengeKeyBase = "twofa:totp:bind"
)

// totpBindChallenge stores one pending TOTP binding attempt in cache.
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
func currentTOTPBindSessionID(ctx *types.ServiceContext) (string, error) {
	if ctx == nil {
		return "", types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}
	if strings.TrimSpace(ctx.SessionID) != "" {
		return strings.TrimSpace(ctx.SessionID), nil
	}
	sessionID, err := ctx.Cookie("session_id")
	if err != nil {
		return "", types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}
	return sessionID, nil
}

// issueTOTPBindChallenge creates a cache-backed challenge for a pending TOTP binding flow.
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
