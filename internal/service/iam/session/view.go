package serviceiamsession

import modeliamsession "github.com/hydroan/gst/internal/model/iam/session"

// buildCurrentSessionView builds the shared response snapshot for session query endpoints.
func buildCurrentSessionView(session modeliamsession.Session, currentSessionID string) modeliamsession.SessionView {
	sessionID := session.ID
	if sessionID == "" {
		sessionID = currentSessionID
	}
	return modeliamsession.SessionView{
		ID:          sessionID,
		State:       session.State,
		IssuedAt:    session.IssuedAt,
		LastSeenAt:  session.LastSeenAt,
		ExpiresAt:   session.ExpiresAt,
		ClientIP:    session.ClientIP,
		UserAgent:   session.UserAgent,
		Platform:    session.Platform,
		OS:          session.OS,
		EngineName:  session.EngineName,
		BrowserName: session.BrowserName,
		IsCurrent:   sessionID == currentSessionID,
	}
}
