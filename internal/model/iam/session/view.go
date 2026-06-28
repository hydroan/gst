package modeliamsession

import "time"

// AdminSessionOwnerView describes a session owner together with all indexed sessions owned by that user.
type AdminSessionOwnerView struct {
	UserID             string        `json:"user_id"`
	Username           string        `json:"username"`
	Email              string        `json:"email"`
	FirstName          *string       `json:"first_name,omitempty"`
	LastName           *string       `json:"last_name,omitempty"`
	Status             string        `json:"status"`
	MustChangePassword bool          `json:"must_change_password"`
	Sessions           []SessionView `json:"sessions"`
}

// SessionView describes a session snapshot returned by session query endpoints.
type SessionView struct {
	ID          string        `json:"id"`
	Status      SessionStatus `json:"status"`
	IssuedAt    time.Time     `json:"issued_at"`
	LastSeenAt  time.Time     `json:"last_seen_at"`
	ExpiresAt   time.Time     `json:"expires_at"`
	ClientIP    string        `json:"client_ip"`
	Platform    string        `json:"platform"`
	OS          string        `json:"os"`
	BrowserName string        `json:"browser_name"`
	IsCurrent   bool          `json:"is_current"`
}

// PrincipalView describes the authenticated principal bound to the current session.
type PrincipalView struct {
	UserID             string  `json:"user_id"`
	Username           string  `json:"username"`
	Email              string  `json:"email"`
	FirstName          *string `json:"first_name,omitempty"`
	LastName           *string `json:"last_name,omitempty"`
	Status             string  `json:"status"`
	MustChangePassword bool    `json:"must_change_password"`
}
