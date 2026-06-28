package modeliamsession

import "time"

// AdminSessionUserView describes a user together with all indexed sessions owned by the user.
type AdminSessionUserView struct {
	UserID             string        `json:"user_id"`
	Username           string        `json:"username"`
	Email              string        `json:"email"`
	FirstName          *string       `json:"first_name,omitempty"`
	LastName           *string       `json:"last_name,omitempty"`
	Status             string        `json:"status"`
	MustChangePassword bool          `json:"must_change_password"`
	SessionTotal       int64         `json:"session_total"`
	Sessions           []SessionView `json:"sessions"`
}

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
