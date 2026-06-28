package serviceiamsession

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types"
)

const (
	sessionTouchInterval        = 30 * time.Second
	sessionLastSeenPruneLockTTL = time.Minute
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
func listUserSessionIDs(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return make([]string, 0), nil
	}
	ctx = redisContext(ctx)
	userKey := modeliamsession.SessionUserKey(userID)
	if err := pruneExpiredSessionIDs(ctx, userKey); err != nil {
		return nil, err
	}
	return redis.ZRange(ctx, userKey, 0, -1)
}

// listAllSessionIDs loads all indexed session ids across users.
//
// The global index has the same lazy-cleanup requirement as the per-user index:
// session payload keys expire independently, while ZSET members remain until we
// remove them. Pruning here makes admin session views count only sessions whose
// index score says they are still within their configured lifetime.
func listAllSessionIDs(ctx context.Context) ([]string, error) {
	ctx = redisContext(ctx)
	if err := pruneExpiredSessionIDs(ctx, modeliamsession.SessionAllKey()); err != nil {
		return nil, err
	}
	return redis.ZRange(ctx, modeliamsession.SessionAllKey(), 0, -1)
}

// listOnlineSessionIDs loads session ids whose last-seen score falls inside the requested window.
//
// SessionLastSeenKey is a global ZSET scored by Session.LastSeenAt in Unix
// milliseconds. This helper intentionally returns only candidate ids; callers
// must still load and validate each session snapshot because the last-seen
// index can outlive expired payload keys or contain ids from partially written
// Redis state.
func listOnlineSessionIDs(ctx context.Context, since time.Time) ([]string, error) {
	if since.IsZero() {
		return make([]string, 0), nil
	}
	ctx = redisContext(ctx)
	pruneStaleLastSeenSessionIDs(ctx, time.Now())
	return redis.ZRangeByScore(
		ctx,
		modeliamsession.SessionLastSeenKey(),
		strconv.FormatInt(since.UnixMilli(), 10),
		"+inf",
	)
}

// pruneExpiredSessionIDs removes expired session ids from a session index ZSET.
//
// It relies on the invariant established by IndexSession: every member's
// score is the session ExpiresAt timestamp in Unix milliseconds. This function
// intentionally only prunes by index score. It does not validate the session
// payload itself; callers still load and validate each remaining payload because
// Redis state can drift after partial writes, manual cache edits, or old data.
func pruneExpiredSessionIDs(ctx context.Context, key string) error {
	return redis.ZRemRangeByScore(redisContext(ctx), key, "-inf", strconv.FormatInt(time.Now().UnixMilli(), 10))
}

// pruneStaleLastSeenSessionIDs bounds the global last-seen index by the maximum session lifetime.
//
// SessionLastSeenKey is scored by LastSeenAt instead of ExpiresAt so online
// queries can search by recent activity. Redis cannot expire individual ZSET
// members when session payload keys naturally expire, so this helper lazily
// removes members whose last-seen timestamp is older than any valid session can
// still be. A short Redis lock keeps high-frequency request paths from pruning
// the same global index on every request.
func pruneStaleLastSeenSessionIDs(ctx context.Context, now time.Time) {
	ctx = redisContext(ctx)
	acquired, err := redis.SetNX(ctx, modeliamsession.SessionLastSeenPruneKey(), "1", sessionLastSeenPruneLockTTL)
	if err != nil || !acquired {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	retention := GetSessionExpiration() + sessionTouchInterval
	cutoff := now.Add(-retention)
	_ = redis.ZRemRangeByScore(ctx, modeliamsession.SessionLastSeenKey(), "-inf", strconv.FormatInt(cutoff.UnixMilli(), 10))
}

// removeSessionIndexes removes a live session id from every Redis index.
//
// The user and global indexes are scored by ExpiresAt and drive ordinary
// session list/delete operations. The last-seen index is scored by LastSeenAt
// and drives online-window queries. Deleting a session must clear all three so
// online queries cannot return revoked or explicitly deleted sessions.
func removeSessionIndexes(ctx context.Context, userID, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if userID != "" {
		if err := redis.ZRem(ctx, modeliamsession.SessionUserKey(userID), sessionID); err != nil {
			return err
		}
	}
	if err := redis.ZRem(ctx, modeliamsession.SessionAllKey(), sessionID); err != nil {
		return err
	}
	return redis.ZRem(ctx, modeliamsession.SessionLastSeenKey(), sessionID)
}

// removeStaleSessionIndexes best-effort removes a stale session id from known Redis indexes.
//
// Stale index entries are expected because Redis expires session payload keys
// independently from ZSET members. Cleanup on read/delete paths must not fail
// the user-facing request when an index member is already stale.
func removeStaleSessionIndexes(ctx context.Context, userID, sessionID string) {
	if sessionID == "" {
		return
	}
	if userID != "" {
		_ = redis.ZRem(ctx, modeliamsession.SessionUserKey(userID), sessionID)
	}
	_ = redis.ZRem(ctx, modeliamsession.SessionAllKey(), sessionID)
	_ = redis.ZRem(ctx, modeliamsession.SessionLastSeenKey(), sessionID)
}

// IndexSession stores a session id in every Redis index used by IAM session queries.
//
// SessionUserKey and SessionAllKey use ExpiresAt as the ZSET score so list
// paths can prune expired ids before loading payloads. SessionLastSeenKey uses
// LastSeenAt as the score so admin online-window queries can avoid scanning all
// active sessions. The session payload key owns the TTL; index cleanup is lazy
// because Redis ZSET members do not expire independently.
func IndexSession(ctx context.Context, session modeliamsession.Session) error {
	if session.UserID == "" || session.ID == "" {
		return nil
	}
	ctx = redisContext(ctx)
	pruneStaleLastSeenSessionIDs(ctx, time.Now())
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return errors.New("session expired")
	}
	// Store the expiration timestamp as the index score. The list paths use this
	// contract to prune expired ZSET members before loading session payloads.
	score := float64(session.ExpiresAt.UnixMilli())
	userKey := modeliamsession.SessionUserKey(session.UserID)
	if err := redis.ZAdd(ctx, userKey, score, session.ID); err != nil {
		return err
	}
	if err := redis.ZAdd(ctx, modeliamsession.SessionAllKey(), score, session.ID); err != nil {
		_ = redis.ZRem(ctx, userKey, session.ID)
		return err
	}
	lastSeenScore := float64(session.LastSeenAt.UnixMilli())
	if err := redis.ZAdd(ctx, modeliamsession.SessionLastSeenKey(), lastSeenScore, session.ID); err != nil {
		removeStaleSessionIndexes(ctx, session.UserID, session.ID)
		return err
	}
	if err := redis.Expire(ctx, userKey, ttl); err != nil {
		removeStaleSessionIndexes(ctx, session.UserID, session.ID)
		return err
	}
	if err := redis.Expire(ctx, modeliamsession.SessionAllKey(), ttl); err != nil {
		removeStaleSessionIndexes(ctx, session.UserID, session.ID)
		return err
	}
	return nil
}

// UpdateSessionMustChangePassword updates the stored session after the user clears MustChangePassword in the database.
func UpdateSessionMustChangePassword(ctx context.Context, sessionID string, mustChange bool) error {
	if sessionID == "" {
		return nil
	}
	ctx = redisContext(ctx)
	cache := redis.Cache[modeliamsession.Session]().WithContext(ctx)
	sessionKey := modeliamsession.SessionIDKey(sessionID)
	session, err := cache.Get(sessionKey)
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil
		}
		return err
	}
	session.MustChangePassword = mustChange
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		_, _ = SessionManager.Delete(ctx, sessionID)
		return types.ErrEntryNotFound
	}
	return cache.Set(sessionKey, session, ttl)
}

// TouchSession refreshes LastSeenAt for an active session at most once per touch interval.
//
// The touch key is a short-lived SetNX lock. It throttles repeated reads of the
// current session and protects against concurrent requests writing older
// snapshots over a fresher LastSeenAt. When a touch is accepted, both the stored
// session snapshot and the global last-seen ZSET are updated with the same time.
func TouchSession(ctx context.Context, sessionID string, session modeliamsession.Session, now time.Time) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	ctx = redisContext(ctx)
	if now.IsZero() {
		now = time.Now()
	}
	if now.Sub(session.LastSeenAt) < sessionTouchInterval {
		return nil
	}

	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		_, _ = SessionManager.Delete(ctx, sessionID)
		return types.ErrEntryNotFound
	}

	touchKey := modeliamsession.SessionTouchKey(sessionID)
	acquired, err := redis.SetNX(ctx, touchKey, "1", sessionTouchInterval)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}

	session.LastSeenAt = now
	if err = redis.Cache[modeliamsession.Session]().
		WithContext(ctx).
		Set(modeliamsession.SessionIDKey(sessionID), session, ttl); err != nil {
		_ = redis.Del(ctx, touchKey)
		return err
	}
	if err = redis.ZAdd(ctx, modeliamsession.SessionLastSeenKey(), float64(now.UnixMilli()), sessionID); err != nil {
		_ = redis.Del(ctx, touchKey)
		return err
	}
	pruneStaleLastSeenSessionIDs(ctx, now)
	return nil
}

// DeleteUserSessionsExceptCurrent deletes all indexed sessions of a user except the current session.
// Missing session records are treated as stale index entries and cleaned up
// from the user's ZSET so the operation remains idempotent.
func DeleteUserSessionsExceptCurrent(ctx context.Context, userID, currentSessionID string) error {
	if userID == "" {
		return nil
	}
	ctx = redisContext(ctx)

	sessionIDs, err := listUserSessionIDs(ctx, userID)
	if err != nil {
		return err
	}

	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" || sessionID == currentSessionID {
			continue
		}

		if _, err = SessionManager.Delete(ctx, sessionID); err != nil {
			if errors.Is(err, types.ErrEntryNotFound) {
				// The session payload may already be gone while the user-session
				// index still references it. Remove the stale index entry and
				// continue deleting the remaining sessions.
				removeStaleSessionIndexes(ctx, userID, sessionID)
				continue
			}
			return err
		}
	}

	return nil
}

// deleteUserSessions deletes all indexed sessions of a user.
// Missing session records are treated as stale index entries and cleaned up
// from the user's ZSET so the operation remains idempotent.
func deleteUserSessions(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	ctx = redisContext(ctx)

	sessionIDs, err := listUserSessionIDs(ctx, userID)
	if err != nil {
		return err
	}

	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" {
			continue
		}

		if _, err = SessionManager.Delete(ctx, sessionID); err != nil {
			if errors.Is(err, types.ErrEntryNotFound) {
				removeStaleSessionIndexes(ctx, userID, sessionID)
				continue
			}
			return err
		}
	}

	return redis.Del(ctx, modeliamsession.SessionUserKey(userID))
}

// InvalidateUserSessions removes all indexed sessions for a user.
// It is best-effort: failures to talk to Redis do not block password updates.
func InvalidateUserSessions(ctx context.Context, userID string) {
	if userID == "" {
		return
	}
	ctx = redisContext(ctx)
	InvalidateUserStateCache(ctx, userID)
	cache := redis.Cache[modeliamsession.Session]().WithContext(ctx)
	sessionIDs, err := listUserSessionIDs(ctx, userID)
	if err == nil {
		for i := range sessionIDs {
			sessionKey := modeliamsession.SessionIDKey(sessionIDs[i])
			_ = cache.Delete(sessionKey)
			removeStaleSessionIndexes(ctx, userID, sessionIDs[i])
		}
	}
	_ = redis.Del(ctx, modeliamsession.SessionUserKey(userID))
}
