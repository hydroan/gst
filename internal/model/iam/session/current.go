package modeliamsession

import "time"

// AuthenticatedSessionRsp returns the authenticated session timing contract and principal snapshot.
type AuthenticatedSessionRsp struct {
	ServerTime time.Time                `json:"server_time"`
	Session    AuthenticatedSessionView `json:"session"`
	Principal  PrincipalView            `json:"principal"`
}

// AuthenticatedSessionView describes the current authenticated session without exposing its bearer session id.
type AuthenticatedSessionView struct {
	State            SessionStatus `json:"state"`
	IssuedAt         time.Time     `json:"issued_at"`
	LastSeenAt       time.Time     `json:"last_seen_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	ExpiresInSeconds int64         `json:"expires_in_seconds"`
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

// CurrentGetReq is the request payload for getting the current session.
type CurrentGetReq struct{}

// CurrentGetRsp returns the current session together with the latest principal snapshot.
type CurrentGetRsp = AuthenticatedSessionRsp

// CurrentDeleteReq is the request payload for deleting the current session.
type CurrentDeleteReq struct{}

// CurrentDeleteRsp is the response payload for deleting the current session.
type CurrentDeleteRsp struct{}
