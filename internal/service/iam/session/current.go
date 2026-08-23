package serviceiamsession

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ValidateSession reports whether a stored snapshot is usable as the session
// the given id names.
//
// It answers from the snapshot alone. Whether the user behind it may still be
// authenticated is a separate question, asked of the database through
// ValidateSessionUserState, because the answer changes without the snapshot
// changing.
func ValidateSession(sessionID string, sessionData modeliamsession.Session) error {
	sessionID = strings.TrimSpace(sessionID)
	switch {
	case sessionID == "":
		return errors.New("session id is required")
	case sessionData.ID != sessionID:
		return errors.New("session id mismatch")
	case sessionData.UserID == "":
		return errors.New("user not authenticated")
	case sessionData.ExpiresAt.IsZero():
		return errors.New("session expiration is required")
	case !sessionData.ExpiresAt.After(time.Now()):
		return errors.New("session expired")
	default:
		return nil
	}
}

// CurrentSession returns the authenticated session of the request.
//
// The middleware has already loaded and validated it and left it on the
// context, so the common path costs nothing; the load below is for callers
// reached without the middleware, and for the case where the context carries a
// different session than the cookie now does.
//
// A snapshot that fails validation is deleted on the way out. It cannot serve
// another request, and leaving it would let every later request pay to load and
// reject it again.
func CurrentSession(ctx *types.ServiceContext) (string, modeliamsession.Session, error) {
	sessionID, err := CookieSessionID(ctx)
	if err != nil {
		return "", modeliamsession.Session{}, err
	}

	if cachedSessionID, sessionData, ok := currentSessionFromContext(ctx); ok && cachedSessionID == sessionID {
		if err = ValidateSession(sessionID, sessionData); err != nil {
			_, _ = Store.DeleteSession(ctx, sessionID)
			return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session invalid", err)
		}
		return sessionID, sessionData, nil
	}

	sessionData, err := Store.LoadSession(ctx, sessionID)
	if err != nil {
		return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session not exists", err)
	}
	if err = ValidateSession(sessionID, sessionData); err != nil {
		_, _ = Store.DeleteSession(ctx, sessionID)
		return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session invalid", err)
	}

	return sessionID, sessionData, nil
}

type currentSessionContextKey struct{}

type currentSessionContextValue struct {
	sessionID string
	session   modeliamsession.Session
}

// WithCurrentSession stores a validated session snapshot on the request context
// so the handlers behind the middleware do not each reload it.
func WithCurrentSession(ctx context.Context, sessionID string, session modeliamsession.Session) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, currentSessionContextKey{}, currentSessionContextValue{
		sessionID: sessionID,
		session:   session,
	})
}

func currentSessionFromContext(ctx context.Context) (string, modeliamsession.Session, bool) {
	if ctx == nil {
		return "", modeliamsession.Session{}, false
	}
	currentSession, ok := ctx.Value(currentSessionContextKey{}).(currentSessionContextValue)
	if !ok || currentSession.sessionID == "" {
		return "", modeliamsession.Session{}, false
	}
	return currentSession.sessionID, currentSession.session, true
}
