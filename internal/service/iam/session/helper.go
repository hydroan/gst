package serviceiamsession

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
)

var (
	sessionExpiration   time.Duration
	sessionExpirationMu sync.RWMutex
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
// If not configured, it returns the default value of 8 hours.
func GetSessionExpiration() time.Duration {
	sessionExpirationMu.RLock()
	defer sessionExpirationMu.RUnlock()
	if sessionExpiration == 0 {
		return 8 * time.Hour
	}
	return sessionExpiration
}

// SetSessionExpiration sets the session expiration time for iam module.
// This function should be called during module registration.
func SetSessionExpiration(expiration time.Duration) {
	sessionExpirationMu.Lock()
	defer sessionExpirationMu.Unlock()
	sessionExpiration = expiration
}
