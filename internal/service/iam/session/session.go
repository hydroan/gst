package serviceiamsession

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

const (
	// SessionCookieName is the HTTP cookie carrying the opaque IAM session id.
	SessionCookieName = "session_id"
	sessionCookiePath = "/"
	sessionIDBytes    = 32
)

var (
	sessionExpiration   time.Duration
	sessionExpirationMu sync.RWMutex
)

// ReadSessionID returns a non-empty trimmed session id from the request cookie.
func ReadSessionID(ctx *types.ServiceContext) (string, error) {
	sessionID, err := ctx.Cookie(SessionCookieName)
	if err != nil {
		return "", service.NewError(http.StatusUnauthorized, err.Error())
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", service.NewError(http.StatusUnauthorized, "session id is required")
	}
	return sessionID, nil
}

// SetSessionCookie writes the current session cookie with hardened defaults.
func SetSessionCookie(ctx *types.ServiceContext, sessionID string, maxAge time.Duration) {
	//nolint:gosec // Secure is derived from TLS/proxy headers; local HTTP cannot set a Secure cookie.
	ctx.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     sessionCookiePath,
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   ctx.IsHTTPS(),
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie removes the current session cookie using the same path and security attributes.
func ClearSessionCookie(ctx *types.ServiceContext) {
	//nolint:gosec // Secure is derived from TLS/proxy headers and must match deployment transport.
	ctx.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     sessionCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   ctx.IsHTTPS(),
		SameSite: http.SameSiteLaxMode,
	})
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
