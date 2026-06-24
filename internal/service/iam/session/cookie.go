package serviceiamsession

import (
	"net/http"
	"strings"
	"time"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

const (
	// SessionCookieName is the HTTP cookie carrying the opaque IAM session id.
	SessionCookieName = "session_id"
	sessionCookiePath = "/"
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
	http.SetCookie(ctx.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     sessionCookiePath,
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   requestUsesHTTPS(ctx.Request),
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie removes the current session cookie using the same path and security attributes.
func ClearSessionCookie(ctx *types.ServiceContext) {
	//nolint:gosec // Secure is derived from TLS/proxy headers and must match deployment transport.
	http.SetCookie(ctx.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     sessionCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestUsesHTTPS(ctx.Request),
		SameSite: http.SameSiteLaxMode,
	})
}

func requestUsesHTTPS(req *http.Request) bool {
	if req == nil {
		return false
	}
	if req.TLS != nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(req.Header.Get("X-Forwarded-Ssl")), "on") {
		return true
	}
	return strings.Contains(strings.ToLower(req.Header.Get("Forwarded")), "proto=https")
}
