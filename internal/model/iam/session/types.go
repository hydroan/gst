package modeliamsession

import (
	"fmt"
	"time"
)

// SessionNamespacePrefix is the shared Redis key prefix for IAM session storage.
const SessionNamespacePrefix = "iam:session"

// SessionIDNamespace stores session snapshots by session ID.
const SessionIDNamespace = SessionNamespacePrefix + ":id"

// SessionUserNamespace stores the session index set by user ID.
const SessionUserNamespace = SessionNamespacePrefix + ":user"

// SessionAllNamespace stores the global session index set by session ID.
const SessionAllNamespace = SessionNamespacePrefix + ":all"

// SessionLastSeenNamespace stores the global last-seen index by session ID.
const SessionLastSeenNamespace = SessionNamespacePrefix + ":last_seen"

// SessionTouchNamespace stores short-lived touch locks by session ID.
const SessionTouchNamespace = SessionNamespacePrefix + ":touch"

// SessionUserStateNamespace stores short-lived mutable user-state cache by user ID.
const SessionUserStateNamespace = SessionNamespacePrefix + ":user_state"

// SessionStatus describes the lifecycle state of an IAM session snapshot.
type SessionStatus string

const (
	// SessionStatusActive marks a session that can still be used.
	SessionStatusActive SessionStatus = "active"
	// SessionStatusRevoked marks a session that was explicitly invalidated.
	SessionStatusRevoked SessionStatus = "revoked"
	// SessionStatusExpired marks a session whose expiration time has passed.
	SessionStatusExpired SessionStatus = "expired"
)

// Session stores the authenticated session snapshot used by IAM middleware and session APIs.
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

	Status     SessionStatus `json:"status"`
	IssuedAt   time.Time     `json:"issued_at"`
	LastSeenAt time.Time     `json:"last_seen_at"`
	ExpiresAt  time.Time     `json:"expires_at"`

	Token Token `json:"token"`
}

// Token stores the token payload associated with an IAM session.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`

	ExpiresIn        int `json:"expires_in"`
	RefreshExpiresIn int `json:"refresh_expires_in"`

	TokenType string `json:"token_type"`
	Scope     string `json:"scope"`

	NotBeforePolicy int    `json:"not-before-policy"`
	SessionState    string `json:"session_state"`
}

// AuthenticatedSessionRsp returns the authenticated session timing contract and principal snapshot.
type AuthenticatedSessionRsp struct {
	ServerTime time.Time                `json:"server_time"`
	Session    AuthenticatedSessionView `json:"session"`
	Principal  PrincipalView            `json:"principal"`
}

// AuthenticatedSessionView describes the current authenticated session without exposing its bearer session id.
type AuthenticatedSessionView struct {
	TenantID         string        `json:"tenant_id,omitempty"`
	Status           SessionStatus `json:"status"`
	IssuedAt         time.Time     `json:"issued_at"`
	LastSeenAt       time.Time     `json:"last_seen_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	ExpiresInSeconds int64         `json:"expires_in_seconds"`
}

// sessionRedisKey builds a Redis key for the specified namespace and identifier.
func sessionRedisKey(namespace, id string) string {
	return fmt.Sprintf("%s:%s", namespace, id)
}

// SessionIDKey builds the Redis key for a session snapshot identified by session ID.
func SessionIDKey(sessionID string) string {
	return sessionRedisKey(SessionIDNamespace, sessionID)
}

// SessionUserKey builds the Redis key for the indexed session set of a user.
func SessionUserKey(userID string) string {
	return sessionRedisKey(SessionUserNamespace, userID)
}

// SessionAllKey builds the Redis key for the indexed session set of all sessions.
func SessionAllKey() string {
	return SessionAllNamespace
}

// SessionLastSeenKey builds the Redis key for the global session last-seen index.
func SessionLastSeenKey() string {
	return SessionLastSeenNamespace
}

// SessionTouchKey builds the Redis key for throttling LastSeenAt updates by session ID.
func SessionTouchKey(sessionID string) string {
	return sessionRedisKey(SessionTouchNamespace, sessionID)
}

// SessionLastSeenPruneKey builds the Redis key for throttling last-seen index pruning.
func SessionLastSeenPruneKey() string {
	return sessionRedisKey(SessionLastSeenNamespace, "prune")
}

// SessionUserStateKey builds the Redis key for cached mutable user state by user ID.
func SessionUserStateKey(userID string) string {
	return sessionRedisKey(SessionUserStateNamespace, userID)
}
