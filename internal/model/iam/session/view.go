package modeliamsession

import "time"

// AdminSessionOwnerView describes a session owner together with all indexed sessions owned by that user.
type AdminSessionOwnerView struct {
	UserID             string        `json:"user_id"`
	Username           string        `json:"username"`
	Email              string        `json:"email"`
	Status             string        `json:"status"`
	MustChangePassword bool          `json:"must_change_password"`
	Sessions           []SessionView `json:"sessions"`
}

// SessionView describes a session snapshot returned by session query endpoints.
type SessionView struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id,omitempty"`
	IssuedAt    time.Time `json:"issued_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	ClientIP    string    `json:"client_ip"`
	Platform    string    `json:"platform"`
	OS          string    `json:"os"`
	BrowserName string    `json:"browser_name"`
	IsCurrent   bool      `json:"is_current"`
}

// PrincipalView describes the authenticated principal bound to the current session.
type PrincipalView struct {
	UserID             string `json:"user_id"`
	Username           string `json:"username"`
	Email              string `json:"email"`
	MustChangePassword bool   `json:"must_change_password"`

	// IsSystemRoot reports whether the principal holds the system-level
	// consts.AUTHZ_SYSTEM_ROLE_ROOT role.
	//
	// It states one fact about who the principal is, not what the principal may
	// do. Every request is decided on its own by authorization and by whatever
	// guard the route itself installs, and a client that reads this as a
	// permission set will disagree with the server as soon as either of them
	// says something this bool cannot express.
	//
	// It is reported because nothing else in this response tells a client
	// whether the session it holds is a system-wide one, and the endpoints that
	// refuse everyone else only say so once the request has been made.
	IsSystemRoot bool `json:"is_system_root"`
}
