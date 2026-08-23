package modeliamsession

import "time"

// Session stores the authenticated session snapshot used by IAM middleware and session APIs.
//
// A stored snapshot is a usable session. Revocation deletes the snapshot and
// expiry lets Redis drop it, so neither has a state left to name: the key is
// either there or it is not, and no reader has to agree with a status field
// about which.
type Session struct {
	ID string `json:"id"`

	UserID             string `json:"user_id"`
	Username           string `json:"username"`
	TenantID           string `json:"tenant_id,omitempty"`
	MustChangePassword bool   `json:"must_change_password"`

	ClientIP    string `json:"client_ip"`
	UserAgent   string `json:"user_agent"`
	Platform    string `json:"platform"`
	OS          string `json:"os"`
	EngineName  string `json:"engine_name"`
	BrowserName string `json:"browser_name"`

	IssuedAt   time.Time `json:"issued_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// AuthenticatedSessionRsp returns the authenticated session timing contract and principal snapshot.
type AuthenticatedSessionRsp struct {
	ServerTime time.Time                `json:"server_time"`
	Session    AuthenticatedSessionView `json:"session"`
	Principal  PrincipalView            `json:"principal"`
}

// AuthenticatedSessionView describes the current authenticated session without exposing its bearer session id.
type AuthenticatedSessionView struct {
	TenantID         string    `json:"tenant_id,omitempty"`
	IssuedAt         time.Time `json:"issued_at"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	ExpiresInSeconds int64     `json:"expires_in_seconds"`
}
