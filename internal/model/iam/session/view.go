package modeliamsession

import "time"

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
