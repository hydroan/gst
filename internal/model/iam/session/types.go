package modeliamsession

import (
	"fmt"
	"time"
)

// The Redis key layout of IAM, in two namespaces.
//
// Every key names the role it plays before it names what it is keyed by, so a
// scan can address one role at a time. That also keeps any key from being a
// prefix of another: a prefix scan for one of them cannot reach into the keys
// of another, which a layout that put the identifier directly under the
// namespace could not promise.
//
// The split between the two namespaces follows what a key is scoped to rather
// than which code writes it. Everything a session owns is reclaimed by dropping
// SessionNamespace; the user-state cache survives the sessions that read it,
// because it describes the user.
//
// Only the two roots are exported. A key is built by the constructor that owns
// it, never by a caller assembling one out of parts, which is what lets the
// layout above change without a caller having to agree to it.
const (
	// SessionNamespace covers every key whose lifetime is a session's.
	SessionNamespace = "iam:session"

	// sessionDataNamespace stores session snapshots by session ID.
	sessionDataNamespace = SessionNamespace + ":data"

	// sessionIndexNamespace covers the sorted sets that index sessions.
	sessionIndexNamespace = SessionNamespace + ":index"

	// sessionIndexUserNamespace indexes a user's sessions by expiry.
	sessionIndexUserNamespace = sessionIndexNamespace + ":user"

	// sessionIndexAllNamespace indexes every session by expiry.
	sessionIndexAllNamespace = sessionIndexNamespace + ":all"

	// sessionIndexSeenNamespace indexes every session by last activity.
	sessionIndexSeenNamespace = sessionIndexNamespace + ":seen"
)

const (
	// UserNamespace covers every key whose lifetime is a user's.
	UserNamespace = "iam:user"

	// userStateNamespace stores the short-lived mutable user-state cache by user ID.
	userStateNamespace = UserNamespace + ":state"
)

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
	TenantID         string    `json:"tenant_id,omitempty"`
	IssuedAt         time.Time `json:"issued_at"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	ExpiresInSeconds int64     `json:"expires_in_seconds"`
}

// namespacedKey builds a Redis key for the specified namespace and identifier.
func namespacedKey(namespace, id string) string {
	return fmt.Sprintf("%s:%s", namespace, id)
}

// SessionDataKey builds the Redis key for a session snapshot identified by session ID.
func SessionDataKey(sessionID string) string {
	return namespacedKey(sessionDataNamespace, sessionID)
}

// SessionIndexUserKey builds the Redis key for the session index of a user.
func SessionIndexUserKey(userID string) string {
	return namespacedKey(sessionIndexUserNamespace, userID)
}

// SessionIndexAllKey builds the Redis key for the session index of all sessions.
func SessionIndexAllKey() string {
	return sessionIndexAllNamespace
}

// SessionIndexSeenKey builds the Redis key for the session index by last activity.
func SessionIndexSeenKey() string {
	return sessionIndexSeenNamespace
}

// UserStateKey builds the Redis key for cached mutable user state by user ID.
func UserStateKey(userID string) string {
	return namespacedKey(userStateNamespace, userID)
}
