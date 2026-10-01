package serviceiamsession

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/service"
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

// MustChangePasswordExempt reports whether the action at the HTTP method and
// path may proceed while the session's MustChangePassword flag is set: the
// ones a user needs to get out of that state, changing the password, logging
// out and reading or ending the current session. Both the HTTP middleware
// and the gRPC interceptor of the module ask it, the gRPC one with the HTTP
// method and route its call's action is served at.
func MustChangePasswordExempt(method, path string) bool {
	switch {
	case method == http.MethodPost && path == "/api/iam/change-password":
		return true
	case method == http.MethodPost && path == "/api/logout":
		return true
	case method == http.MethodGet && path == "/api/iam/session/current":
		return true
	case method == http.MethodDelete && path == "/api/iam/session/current":
		return true
	default:
		return false
	}
}

// CurrentSession returns the authenticated session of the request or call.
//
// The session the middleware or interceptor admitted is on the context (see
// WithCurrentSession) and answers first, on either listener: a gRPC call has
// no cookie to read, and asking its context for one is a call the listener
// refuses once the action returns. The cookie is read for a request reached
// without the middleware, whose session is then loaded through loadSession: a
// store that does not answer is 500, not 401, and a stored value that is not
// a snapshot is deleted and 401.
//
// A snapshot that fails validation is deleted on the way out. It cannot serve
// another request, and leaving it would let every later request pay to load and
// reject it again.
func CurrentSession(ctx *gst.ServiceContext) (string, modeliamsession.Session, error) {
	if sessionID, sessionData, ok := currentSessionFromContext(ctx); ok {
		if err := ValidateSession(sessionID, sessionData); err != nil {
			_, _ = Store.DeleteSession(ctx, sessionID)
			return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session invalid", err)
		}
		return sessionID, sessionData, nil
	}

	sessionID, err := CookieSessionID(ctx)
	if err != nil {
		return "", modeliamsession.Session{}, err
	}
	sessionData, found, err := loadSession(ctx, sessionID)
	if err != nil {
		return "", modeliamsession.Session{}, err
	}
	if !found {
		return "", modeliamsession.Session{}, service.NewError(http.StatusUnauthorized, "session not exists")
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
