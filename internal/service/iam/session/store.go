package serviceiamsession

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// listUserSessionIDs loads all indexed session ids for a user.
//
// Session indexes are stored as Redis ZSETs. Redis can automatically expire the
// session payload key (iam:session:id:<sessionID>), but it cannot automatically
// remove that sessionID from the user/global ZSET indexes. IndexSession uses
// ExpiresAt.UnixMilli() as the ZSET score, so this read path first removes
// members whose score is already in the past. That keeps session list totals
// from being inflated by stale index entries and avoids unnecessary payload GETs
// for sessions that are known to have expired.
func listUserSessionIDs(userID string) ([]string, error) {
	if userID == "" {
		return make([]string, 0), nil
	}
	userKey := modeliamsession.SessionUserKey(userID)
	if err := pruneExpiredSessionIDs(userKey); err != nil {
		return nil, err
	}
	return redis.ZRange(userKey, 0, -1)
}

// listAllSessionIDs loads all indexed session ids across users.
//
// The global index has the same lazy-cleanup requirement as the per-user index:
// session payload keys expire independently, while ZSET members remain until we
// remove them. Pruning here makes admin session views count only sessions whose
// index score says they are still within their configured lifetime.
func listAllSessionIDs() ([]string, error) {
	if err := pruneExpiredSessionIDs(modeliamsession.SessionAllKey()); err != nil {
		return nil, err
	}
	return redis.ZRange(modeliamsession.SessionAllKey(), 0, -1)
}

// pruneExpiredSessionIDs removes expired session ids from a session index ZSET.
//
// It relies on the invariant established by IndexSession: every member's
// score is the session ExpiresAt timestamp in Unix milliseconds. This function
// intentionally only prunes by index score. It does not validate the session
// payload itself; callers still load and validate each remaining payload because
// Redis state can drift after partial writes, manual cache edits, or old data.
func pruneExpiredSessionIDs(key string) error {
	return redis.ZRemRangeByScore(key, "-inf", strconv.FormatInt(time.Now().UnixMilli(), 10))
}

// GetCurrentSession loads and validates the current authenticated user session
// from the request cookie and Redis storage. Database-level checks such as user
// status, permission, or account existence remain the responsibility of the caller.
func GetCurrentSession(ctx *types.ServiceContext) (string, modeliamsession.Session, error) {
	sessionID, err := ReadSessionID(ctx)
	if err != nil {
		return "", modeliamsession.Session{}, err
	}

	session, err := LoadSession(sessionID)
	if err != nil {
		return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session not exists", err)
	}
	if err = ValidateActiveSession(sessionID, session); err != nil {
		_, _ = DeleteSession(sessionID)
		return "", modeliamsession.Session{}, service.NewErrorWithCause(http.StatusUnauthorized, "session invalid", err)
	}

	return sessionID, session, nil
}

// LoadSession loads the Redis session snapshot for a session id.
func LoadSession(sessionID string) (modeliamsession.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return modeliamsession.Session{}, types.ErrEntryNotFound
	}
	return redis.Cache[modeliamsession.Session]().Get(modeliamsession.SessionIDKey(sessionID))
}

// ValidateActiveSession verifies that a Redis session snapshot is the active session for the given id.
func ValidateActiveSession(sessionID string, session modeliamsession.Session) error {
	sessionID = strings.TrimSpace(sessionID)
	switch {
	case sessionID == "":
		return errors.New("session id is required")
	case session.ID != sessionID:
		return errors.New("session id mismatch")
	case session.UserID == "":
		return errors.New("user not authenticated")
	case session.State != modeliamsession.SessionStatusActive:
		return errors.New("session is not active")
	case session.ExpiresAt.IsZero():
		return errors.New("session expiration is required")
	case !session.ExpiresAt.After(time.Now()):
		return errors.New("session expired")
	default:
		return nil
	}
}

// IndexSession adds the session id into the user and global session indexes.
func IndexSession(session modeliamsession.Session) error {
	if session.UserID == "" || session.ID == "" {
		return nil
	}
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return errors.New("session expired")
	}
	// Store the expiration timestamp as the index score. The list paths use this
	// contract to prune expired ZSET members before loading session payloads.
	score := float64(session.ExpiresAt.UnixMilli())
	userKey := modeliamsession.SessionUserKey(session.UserID)
	if err := redis.ZAdd(userKey, score, session.ID); err != nil {
		return err
	}
	if err := redis.ZAdd(modeliamsession.SessionAllKey(), score, session.ID); err != nil {
		_ = redis.ZRem(userKey, session.ID)
		return err
	}
	if err := redis.Expire(userKey, ttl); err != nil {
		_ = redis.ZRem(userKey, session.ID)
		_ = redis.ZRem(modeliamsession.SessionAllKey(), session.ID)
		return err
	}
	if err := redis.Expire(modeliamsession.SessionAllKey(), ttl); err != nil {
		_ = redis.ZRem(userKey, session.ID)
		_ = redis.ZRem(modeliamsession.SessionAllKey(), session.ID)
		return err
	}
	return nil
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
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		_, _ = DeleteSession(sessionID)
		return types.ErrEntryNotFound
	}
	return redis.Cache[modeliamsession.Session]().Set(sessionKey, session, ttl)
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
