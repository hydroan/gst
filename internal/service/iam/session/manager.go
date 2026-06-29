package serviceiamsession

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

const (
	// SessionCookieName is the HTTP cookie carrying the opaque IAM session id.
	SessionCookieName = "session_id"
	sessionCookiePath = "/"
	sessionIDBytes    = 32
)

var SessionManager = sessionManager{}

type sessionManager struct{}

// SessionID returns a non-empty trimmed session id from the request cookie.
func (sessionManager) SessionID(ctx *types.ServiceContext) (string, error) {
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

// Validate verifies that a Redis session snapshot is the active session for the given id.
func (sessionManager) Validate(sessionID string, sessionData modeliamsession.Session) error {
	sessionID = strings.TrimSpace(sessionID)
	switch {
	case sessionID == "":
		return errors.New("session id is required")
	case sessionData.ID != sessionID:
		return errors.New("session id mismatch")
	case sessionData.UserID == "":
		return errors.New("user not authenticated")
	case sessionData.Status != modeliamsession.SessionStatusActive:
		return errors.New("session is not active")
	case sessionData.ExpiresAt.IsZero():
		return errors.New("session expiration is required")
	case !sessionData.ExpiresAt.After(time.Now()):
		return errors.New("session expired")
	default:
		return nil
	}
}

// Load loads the Redis session snapshot for a session id.
func (sessionManager) Load(ctx context.Context, sessionID string) (modeliamsession.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return modeliamsession.Session{}, types.ErrEntryNotFound
	}
	return redis.Cache[modeliamsession.Session]().WithContext(redisContext(ctx)).Get(modeliamsession.SessionIDKey(sessionID))
}

// Delete deletes the stored session payload and removes it from every Redis index.
func (sessionManager) Delete(ctx context.Context, sessionID string) (modeliamsession.Session, error) {
	if sessionID == "" {
		return modeliamsession.Session{}, nil
	}
	ctx = redisContext(ctx)
	cache := redis.Cache[modeliamsession.Session]().WithContext(ctx)

	sessionKey := modeliamsession.SessionIDKey(sessionID)
	sessionData, err := cache.Get(sessionKey)
	if err != nil {
		return modeliamsession.Session{}, err
	}
	if err = cache.Delete(sessionKey); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
		return sessionData, err
	}

	if err = removeSessionIndexes(ctx, sessionData.UserID, sessionID); err != nil {
		return sessionData, err
	}

	return sessionData, nil
}

// Current loads and validates the current authenticated user session
// from the request cookie and Redis storage. Database-level checks such as user
// status, permission, or account existence remain the responsibility of the caller.
func (sessionManager) Current(ctx *types.ServiceContext) (string, modeliamsession.Session, error) {
	sessionID, err := SessionManager.SessionID(ctx)
	if err != nil {
		return "", modeliamsession.Session{}, err
	}

	if cachedSessionID, sessionData, ok := currentSessionFromContext(ctx); ok && cachedSessionID == sessionID {
		if err = SessionManager.Validate(sessionID, sessionData); err != nil {
			_, _ = SessionManager.Delete(ctx, sessionID)
			return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session invalid", err)
		}
		return sessionID, sessionData, nil
	}

	sessionData, err := SessionManager.Load(ctx, sessionID)
	if err != nil {
		return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session not exists", err)
	}
	if err = SessionManager.Validate(sessionID, sessionData); err != nil {
		_, _ = SessionManager.Delete(ctx, sessionID)
		return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session invalid", err)
	}

	return sessionID, sessionData, nil
}

// SetCookie writes the current session cookie with hardened defaults.
func (sessionManager) SetCookie(ctx *types.ServiceContext, sessionID string, maxAge time.Duration) {
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

// ClearCookie removes the current session cookie using the same path and security attributes.
func (sessionManager) ClearCookie(ctx *types.ServiceContext) {
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
