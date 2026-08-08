package modeliamsession

import (
	"context"

	"github.com/hydroan/gst/redis"
)

// InvalidateUserSessions revokes every session a user holds.
//
// User lifecycle operations - deleting a user, locking an account, forcing a
// logout - happen wherever a project models its users, which is regularly
// outside the IAM module's own service package. Session storage is reachable
// from here, the package that owns the Redis key layout, so any service package
// can revoke sessions without reaching across service module boundaries.
//
// Authentication reads two things: the session snapshot addressed by session id,
// and the short-lived user-state cache that keeps hot requests off the database.
// Both have to go, or a revoked user keeps passing authentication until that
// cache expires on its own.
//
// Best-effort by design: callers run this after a state change that already
// happened, so a Redis failure must not fail their request. The user-state cache
// expiring and refreshing from the database is the backstop.
func InvalidateUserSessions(ctx context.Context, userID string) {
	if userID == "" {
		return
	}
	ctx = redisContext(ctx)
	InvalidateUserStateCache(ctx, userID)

	userKey := SessionUserKey(userID)
	sessionIDs, _ := redis.ZRange(ctx, userKey, 0, -1)
	for i := range sessionIDs {
		if sessionIDs[i] == "" {
			continue
		}
		_ = redis.Del(ctx, SessionIDKey(sessionIDs[i]))
		_ = RemoveSessionIndexes(ctx, userID, sessionIDs[i])
	}
	_ = redis.Del(ctx, userKey)
}

// InvalidateUserStateCache drops the cached mutable user state of a user.
//
// The cache answers "may this session keep going" without a database round trip,
// so anything that changes a user's status, credentials or existence has to drop
// it; otherwise the change only takes effect once the entry expires.
func InvalidateUserStateCache(ctx context.Context, userID string) {
	if userID == "" {
		return
	}
	_ = redis.Del(redisContext(ctx), SessionUserStateKey(userID))
}

// RemoveSessionIndexes removes a session id from every index pointing at it.
//
// The user and global indexes are scored by ExpiresAt and drive session
// list/delete operations. The last-seen index is scored by LastSeenAt and drives
// online-window queries. A session going away has to clear all three, or online
// queries keep returning sessions that no longer exist. An empty userID skips
// the per-user index, for callers holding nothing but a stale global index
// member. Callers cleaning up stale members discard the error; callers deleting
// a live session report it.
func RemoveSessionIndexes(ctx context.Context, userID, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	ctx = redisContext(ctx)
	if userID != "" {
		if err := redis.ZRem(ctx, SessionUserKey(userID), sessionID); err != nil {
			return err
		}
	}
	if err := redis.ZRem(ctx, SessionAllKey(), sessionID); err != nil {
		return err
	}
	return redis.ZRem(ctx, SessionLastSeenKey(), sessionID)
}

// redisContext keeps a nil context from reaching the Redis client.
func redisContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
