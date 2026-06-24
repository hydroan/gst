package serviceiamsession

import (
	"net/http"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var (
	sessionExpiration   time.Duration
	sessionExpirationMu sync.RWMutex
)

// listUserSessionIDs loads all indexed session ids for a user.
func listUserSessionIDs(userID string) ([]string, error) {
	if userID == "" {
		return make([]string, 0), nil
	}
	userKey := modeliamsession.SessionUserKey(userID)
	return redis.ZRange(userKey, 0, -1)
}

// listAllSessionIDs loads all indexed session ids across users.
func listAllSessionIDs() ([]string, error) {
	return redis.ZRange(modeliamsession.SessionAllKey(), 0, -1)
}

// GetCurrentSession loads the current authenticated user session from the
// request cookie and Redis storage. It only enforces the minimal integrity
// required by IAM services: the session must exist and be bound to a user.
// Database-level checks such as user status, permission, or account existence
// remain the responsibility of the caller.
func GetCurrentSession(ctx *types.ServiceContext) (string, modeliamsession.Session, error) {
	sessionID, err := ctx.Cookie("session_id")
	if err != nil {
		return "", modeliamsession.Session{}, service.NewError(http.StatusUnauthorized, err.Error())
	}

	sessionKey := modeliamsession.SessionIDKey(sessionID)
	session, err := redis.Cache[modeliamsession.Session]().Get(sessionKey)
	if err != nil {
		return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session not exists", err)
	}
	// An IAM current-session lookup must resolve to an authenticated user
	// session. An empty UserID indicates incomplete or stale session data.
	if session.UserID == "" {
		return "", modeliamsession.Session{}, service.NewError(http.StatusUnauthorized, "user not authenticated")
	}

	return sessionID, session, nil
}

// TrackUserSession adds the session id into the user's indexed session set.
func TrackUserSession(session modeliamsession.Session) error {
	if session.UserID == "" || session.ID == "" {
		return nil
	}
	score := float64(session.IssuedAt.UnixMilli())
	userKey := modeliamsession.SessionUserKey(session.UserID)
	if err := redis.ZAdd(userKey, score, session.ID); err != nil {
		return err
	}
	if err := redis.ZAdd(modeliamsession.SessionAllKey(), score, session.ID); err != nil {
		return err
	}
	expiration := GetSessionExpiration()
	if err := redis.Expire(userKey, expiration); err != nil {
		return err
	}
	return redis.Expire(modeliamsession.SessionAllKey(), expiration)
}

// UpdateSessionMustChangePassword updates the stored session after the user clears MustChangePassword in the database.
func UpdateSessionMustChangePassword(sessionID string, mustChange bool) error {
	if sessionID == "" {
		return nil
	}
	sessionKey := modeliamsession.SessionIDKey(sessionID)
	session, err := redis.Cache[modeliamsession.Session]().Get(sessionKey)
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil
		}
		return err
	}
	session.MustChangePassword = mustChange
	return redis.Cache[modeliamsession.Session]().Set(sessionKey, session, GetSessionExpiration())
}

// DeleteSession deletes the stored session and removes the indexed user-session relation.
func DeleteSession(sessionID string) (modeliamsession.Session, error) {
	if sessionID == "" {
		return modeliamsession.Session{}, nil
	}

	sessionKey := modeliamsession.SessionIDKey(sessionID)
	session, err := redis.Cache[modeliamsession.Session]().Get(sessionKey)
	if err != nil {
		return modeliamsession.Session{}, err
	}
	if err = redis.Cache[modeliamsession.Session]().Delete(sessionKey); err != nil && !errors.Is(err, types.ErrEntryNotFound) {
		return session, err
	}

	if session.UserID != "" {
		userKey := modeliamsession.SessionUserKey(session.UserID)
		if err = redis.ZRem(userKey, sessionID); err != nil {
			return session, err
		}
	}
	if err = redis.ZRem(modeliamsession.SessionAllKey(), sessionID); err != nil {
		return session, err
	}

	return session, nil
}

// DeleteOtherSessions deletes all indexed sessions of a user except the current session.
// Missing session records are treated as stale index entries and cleaned up
// from the user's ZSET so the operation remains idempotent.
func DeleteOtherSessions(userID, currentSessionID string) error {
	if userID == "" {
		return nil
	}

	sessionIDs, err := listUserSessionIDs(userID)
	if err != nil {
		return err
	}

	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" || sessionID == currentSessionID {
			continue
		}

		if _, err = DeleteSession(sessionID); err != nil {
			if errors.Is(err, types.ErrEntryNotFound) {
				// The session payload may already be gone while the user-session
				// index still references it. Remove the stale index entry and
				// continue deleting the remaining sessions.
				_ = redis.ZRem(modeliamsession.SessionUserKey(userID), sessionID)
				_ = redis.ZRem(modeliamsession.SessionAllKey(), sessionID)
				continue
			}
			return err
		}
	}

	return nil
}

// DeleteAllSessions deletes all indexed sessions of a user.
// Missing session records are treated as stale index entries and cleaned up
// from the user's ZSET so the operation remains idempotent.
func DeleteAllSessions(userID string) error {
	if userID == "" {
		return nil
	}

	sessionIDs, err := listUserSessionIDs(userID)
	if err != nil {
		return err
	}

	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" {
			continue
		}

		if _, err = DeleteSession(sessionID); err != nil {
			if errors.Is(err, types.ErrEntryNotFound) {
				_ = redis.ZRem(modeliamsession.SessionUserKey(userID), sessionID)
				_ = redis.ZRem(modeliamsession.SessionAllKey(), sessionID)
				continue
			}
			return err
		}
	}

	return redis.Del(modeliamsession.SessionUserKey(userID))
}

// InvalidateUserSessions removes all indexed sessions for a user.
// It is best-effort: failures to talk to Redis do not block password updates.
func InvalidateUserSessions(userID string) {
	if userID == "" {
		return
	}
	sessionIDs, err := listUserSessionIDs(userID)
	if err == nil {
		for i := range sessionIDs {
			sessionKey := modeliamsession.SessionIDKey(sessionIDs[i])
			_ = redis.Cache[modeliamsession.Session]().Delete(sessionKey)
			_ = redis.ZRem(modeliamsession.SessionAllKey(), sessionIDs[i])
		}
	}
	_ = redis.Del(modeliamsession.SessionUserKey(userID))
}

// GetSessionExpiration returns the configured session expiration time.
// If not configured, it returns the default value of 8 hours.
func GetSessionExpiration() time.Duration {
	sessionExpirationMu.RLock()
	defer sessionExpirationMu.RUnlock()
	if sessionExpiration == 0 {
		return 8 * time.Hour
	}
	return sessionExpiration
}

// SetSessionExpiration sets the session expiration time for iam module.
// This function should be called during module registration.
func SetSessionExpiration(expiration time.Duration) {
	sessionExpirationMu.Lock()
	defer sessionExpirationMu.Unlock()
	sessionExpiration = expiration
}
