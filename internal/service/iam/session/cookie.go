package serviceiamsession

import (
	"net/http"
	"strings"
	"time"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// CookieSessionID returns the session id carried by the request cookie.
//
// A request without one is not an anonymous request this layer can shrug at:
// every caller of this function is already inside an authenticated route, so a
// missing cookie is an unauthenticated caller and is reported as one.
func CookieSessionID(ctx *types.ServiceContext) (string, error) {
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

// SetCookie writes the session cookie with hardened defaults.
func SetCookie(ctx *types.ServiceContext, sessionID string, maxAge time.Duration) {
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

// ClearCookie removes the session cookie using the same path and security
// attributes SetCookie wrote it with, which is what makes the browser drop the
// cookie rather than keep a second one.
func ClearCookie(ctx *types.ServiceContext) {
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
