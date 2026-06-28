package serviceiamsession

import (
	"context"
	"strings"

	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
)

type currentSessionContextKey struct{}

type currentSessionContextValue struct {
	sessionID string
	session   modeliamsession.Session
}

// WithCurrentSession stores a validated IAM session snapshot in the request context.
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
