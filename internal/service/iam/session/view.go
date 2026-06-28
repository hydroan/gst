package serviceiamsession

import (
	"time"

	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
)

// buildSessionView builds the shared response snapshot for session query endpoints.
func buildSessionView(session modeliamsession.Session, currentSessionID string) modeliamsession.SessionView {
	sessionID := session.ID
	if sessionID == "" {
		sessionID = currentSessionID
	}
	return modeliamsession.SessionView{
		ID:          sessionID,
		Status:      session.Status,
		IssuedAt:    session.IssuedAt,
		LastSeenAt:  session.LastSeenAt,
		ExpiresAt:   session.ExpiresAt,
		ClientIP:    session.ClientIP,
		Platform:    session.Platform,
		OS:          session.OS,
		BrowserName: session.BrowserName,
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
