package serviceiamsession

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
)

func redisContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// ensureSessionUserActive verifies that the authenticated user can keep using an existing session.
func ensureSessionUserActive(targetUser *modeliamuser.User) error {
	switch targetUser.Status {
	case modeliamuser.UserStatusInactive:
		return service.NewError(http.StatusForbidden, "account disabled")
	case modeliamuser.UserStatusLocked:
		return service.NewError(http.StatusForbidden, "account locked")
	default:
		return nil
	}
}

// buildSessionView builds the shared response snapshot for session query endpoints.
func buildSessionView(sessionData modeliamsession.Session, currentSessionID string) modeliamsession.SessionView {
	sessionID := sessionData.ID
	if sessionID == "" {
		sessionID = currentSessionID
	}
	return modeliamsession.SessionView{
		ID:          sessionID,
		TenantID:    sessionData.TenantID,
		Status:      sessionData.Status,
		IssuedAt:    sessionData.IssuedAt,
		LastSeenAt:  sessionData.LastSeenAt,
		ExpiresAt:   sessionData.ExpiresAt,
		ClientIP:    sessionData.ClientIP,
		Platform:    sessionData.Platform,
		OS:          sessionData.OS,
		BrowserName: sessionData.BrowserName,
		IsCurrent:   sessionID == currentSessionID,
	}
}

// sessionViewActiveAt returns the timestamp used for stable session ordering.
func sessionViewActiveAt(view modeliamsession.SessionView) time.Time {
	if !view.LastSeenAt.IsZero() {
		return view.LastSeenAt
	}
	return view.IssuedAt
}

// NewSessionID returns an opaque random session identifier.
func NewSessionID() (string, error) {
	buf := make([]byte, sessionIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// GetSessionExpiration returns the configured session expiration time.
// If not configured, it reads IAM_SESSION_EXPIRATION before falling back to 8 hours.
func GetSessionExpiration() time.Duration {
	sessionExpirationMu.RLock()
	expiration := sessionExpiration
	sessionExpirationMu.RUnlock()

	return resolveSessionExpiration(expiration)
}

// SetSessionExpiration sets the session expiration time for iam module.
// This function should be called during module registration.
func SetSessionExpiration(expiration time.Duration) {
	if expiration < 0 {
		panic(errors.New("SessionExpiration must be greater than 0"))
	}

	sessionExpirationMu.Lock()
	defer sessionExpirationMu.Unlock()
	sessionExpiration = expiration
}

func resolveSessionExpiration(expiration time.Duration) time.Duration {
	if expiration > 0 {
		return expiration
	}

	raw := strings.TrimSpace(os.Getenv(sessionExpirationEnv))
	if raw == "" {
		return defaultSessionExpiration
	}

	duration, err := time.ParseDuration(raw)
	if err != nil {
		panic(errors.Wrapf(err, "invalid %s", sessionExpirationEnv))
	}
	if duration <= 0 {
		panic(errors.Errorf("%s must be greater than 0", sessionExpirationEnv))
	}
	return duration
}
