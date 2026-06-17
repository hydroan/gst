package modeliamsession

import (
	"time"
)

// CurrentListReq is the request payload for getting the current session.
type CurrentListReq struct{}

// CurrentListRsp returns the current session together with the latest principal snapshot.
type CurrentListRsp struct {
	Session   SessionView      `json:"session"`
	Principal CurrentPrincipal `json:"principal"`
}

// CurrentDeleteReq is the request payload for deleting the current session.
type CurrentDeleteReq struct{}

// CurrentDeleteRsp is the response payload for deleting the current session.
type CurrentDeleteRsp struct{}

// SessionView describes a session snapshot returned by session query endpoints.
type SessionView struct {
	ID          string        `json:"id"`
	State       SessionStatus `json:"state"`
	IssuedAt    time.Time     `json:"issued_at"`
	LastSeenAt  time.Time     `json:"last_seen_at"`
	ExpiresAt   time.Time     `json:"expires_at"`
	ClientIP    string        `json:"client_ip"`
	UserAgent   string        `json:"user_agent"`
	Platform    string        `json:"platform"`
	OS          string        `json:"os"`
	EngineName  string        `json:"engine_name"`
	BrowserName string        `json:"browser_name"`
	IsCurrent   bool          `json:"is_current"`
}

// CurrentPrincipal describes the authenticated principal bound to the current session.
type CurrentPrincipal struct {
	UserID             string  `json:"user_id"`
	Username           string  `json:"username"`
	Email              string  `json:"email"`
	FirstName          *string `json:"first_name,omitempty"`
	LastName           *string `json:"last_name,omitempty"`
	GroupID            string  `json:"group_id,omitempty"`
	GroupName          string  `json:"group_name,omitempty"`
	Status             string  `json:"status"`
	MustChangePassword bool    `json:"must_change_password"`
}
