package serviceiamsession

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

const sessionTouchInterval = 30 * time.Second

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
	userKey := modeliamsession.SessionUserKey(userID)
	if err := pruneIndex(ctx, userKey, time.Now()); err != nil {
		return nil, err
	}
	sessionIDs, err := redis.ZRange(ctx, userKey, 0, -1)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list user sessions", err)
	}
	return sessionIDs, nil
}

// listAllSessionIDs loads all indexed session ids across users.
//
// The global index has the same lazy-cleanup requirement as the per-user index:
// session payload keys expire independently, while ZSET members remain until we
// remove them. Pruning here makes admin session views count only sessions whose
// index score says they are still within their configured lifetime.
func listAllSessionIDs(ctx context.Context) ([]string, error) {
	if err := pruneIndex(ctx, modeliamsession.SessionAllKey(), time.Now()); err != nil {
		return nil, err
	}
	sessionIDs, err := redis.ZRange(ctx, modeliamsession.SessionAllKey(), 0, -1)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list sessions", err)
	}
	return sessionIDs, nil
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
	// Sweeping is hygiene for a query that already filters by score, so its
	// failure is not this caller's answer to give.
	_ = pruneIndex(ctx, modeliamsession.SessionLastSeenKey(), seenIndexCutoff(time.Now()))
	sessionIDs, err := redis.ZRangeByScore(
		ctx,
		modeliamsession.SessionLastSeenKey(),
		strconv.FormatInt(since.UnixMilli(), 10),
		"+inf",
	)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list online sessions", err)
	}
	return sessionIDs, nil
}

// pruneIndex removes members scored before cutoff from a session index ZSET.
//
// Redis expires whole keys, never individual ZSET members, so a session whose
// payload key has already expired leaves its id behind in every index pointing
// at it. Every index is therefore swept on the paths that read or extend it,
// which is what keeps session counts from being inflated by ids that resolve to
// nothing.
//
// The cutoff is what the caller's index means by stale, because the two index
// families are scored differently: the user and global indexes carry ExpiresAt,
// so anything scored before now is gone; the last-seen index carries
// LastSeenAt, so staleness is bounded by seenIndexCutoff instead.
//
// Pruning by score alone does not make the remaining members trustworthy.
// Callers still load and validate each payload, because Redis state can drift
// after a partial write or an edit made outside this package.
func pruneIndex(ctx context.Context, key string, cutoff time.Time) error {
	if err := redis.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(cutoff.UnixMilli(), 10)); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to prune expired sessions", err)
	}
	return nil
}

// indexRetention is how long an index key has to outlive the write that last
// extended it before nothing it holds can describe a live session.
//
// A session cannot outlive the configured lifetime, so an index scored by
// ExpiresAt is spent one lifetime after its newest member was added.
func indexRetention() time.Duration {
	return GetSessionExpiration()
}

// seenIndexRetention is the same span for the last-seen index, which is scored
// by activity rather than by expiry.
//
// A live session's LastSeenAt lags the present by at most one touch interval,
// because that is how often it is refreshed, so the last-seen index stays
// meaningful for one interval longer than the session lifetime itself.
func seenIndexRetention() time.Duration {
	return GetSessionExpiration() + sessionTouchInterval
}

// seenIndexCutoff returns the last-seen timestamp before which a member of the
// last-seen index cannot belong to any session that is still alive. It is the
// member-level counterpart of seenIndexRetention, and both read the same span
// so a member and the key holding it cannot disagree about staleness.
func seenIndexCutoff(now time.Time) time.Time {
	return now.Add(-seenIndexRetention())
}

// removeSessionIndexes removes a live session id from every Redis index.
//
// Deleting a live session is a user-facing operation, so an index that refuses
// to give the session up is reported rather than swallowed. Read paths cleaning
// up members they already know are stale call the model helper directly and
// discard its error.
func removeSessionIndexes(ctx context.Context, userID, sessionID string) error {
	if err := modeliamsession.RemoveSessionIndexes(ctx, userID, sessionID); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to remove session index", err)
	}
	return nil
}

// IndexSession stores a session id in every Redis index used by IAM session queries.
//
// SessionUserKey and SessionAllKey use ExpiresAt as the ZSET score so list
// paths can prune expired ids before loading payloads. SessionLastSeenKey uses
// LastSeenAt as the score so admin online-window queries can avoid scanning all
// active sessions.
//
// Pruning keeps each index's contents honest, and the ttl set here keeps the
// key itself from outliving every session it names. The two are not
// interchangeable: Redis expires whole keys and never individual ZSET members,
// so without the sweep an index accumulates ids that resolve to nothing, and
// without the ttl an index nobody reads is never reclaimed at all.
//
// Every index is a shared key, so its lifetime is a property of the module and
// not of the session being written: it comes from the configured session
// lifetime rather than from this session's remaining time. Deriving it from the
// member would let a session with little time left cut short a key that older,
// longer-lived members still depend on.
func IndexSession(ctx context.Context, sessionData modeliamsession.Session) error {
	if sessionData.UserID == "" || sessionData.ID == "" {
		return nil
	}
	// A login is the one moment cheap enough to sweep the global last-seen index
	// on; failing to sweep is no reason to fail the login.
	_ = pruneIndex(ctx, modeliamsession.SessionLastSeenKey(), seenIndexCutoff(time.Now()))
	if time.Until(sessionData.ExpiresAt) <= 0 {
		return service.NewError(http.StatusInternalServerError, "session expired")
	}
	// Store the expiration timestamp as the index score. The list paths use this
	// contract to prune expired ZSET members before loading session payloads.
	score := float64(sessionData.ExpiresAt.UnixMilli())
	userKey := modeliamsession.SessionUserKey(sessionData.UserID)
	if err := redis.ZAdd(ctx, userKey, score, sessionData.ID); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to index session", err)
	}
	if err := redis.ZAdd(ctx, modeliamsession.SessionAllKey(), score, sessionData.ID); err != nil {
		_ = redis.ZRem(ctx, userKey, sessionData.ID)
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to index session", err)
	}
	lastSeenScore := float64(sessionData.LastSeenAt.UnixMilli())
	if err := redis.ZAdd(ctx, modeliamsession.SessionLastSeenKey(), lastSeenScore, sessionData.ID); err != nil {
		_ = modeliamsession.RemoveSessionIndexes(ctx, sessionData.UserID, sessionData.ID)
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to index session", err)
	}
	if err := redis.Expire(ctx, userKey, indexRetention()); err != nil {
		_ = modeliamsession.RemoveSessionIndexes(ctx, sessionData.UserID, sessionData.ID)
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to index session", err)
	}
	if err := redis.Expire(ctx, modeliamsession.SessionAllKey(), indexRetention()); err != nil {
		_ = modeliamsession.RemoveSessionIndexes(ctx, sessionData.UserID, sessionData.ID)
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to index session", err)
	}
	if err := redis.Expire(ctx, modeliamsession.SessionLastSeenKey(), seenIndexRetention()); err != nil {
		_ = modeliamsession.RemoveSessionIndexes(ctx, sessionData.UserID, sessionData.ID)
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to index session", err)
	}
	return nil
}

// TouchSession refreshes LastSeenAt for an active session at most once per touch interval.
//
// The interval is enforced against the snapshot the caller already holds, so a
// request inside the interval costs nothing: the comparison happens before any
// Redis command is sent, and that is what keeps a per-request activity stamp
// from turning into a per-request snapshot rewrite.
//
// Concurrent requests that all read the same interval-old snapshot each write,
// rather than one of them taking a lock and the rest standing down. They write
// timestamps milliseconds apart into a field nothing reads for ordering, so the
// last write winning is the whole of the resolution the field needs.
func TouchSession(ctx context.Context, sessionID string, sessionData modeliamsession.Session, now time.Time) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	if now.Sub(sessionData.LastSeenAt) < sessionTouchInterval {
		return nil
	}

	ttl := time.Until(sessionData.ExpiresAt)
	if ttl <= 0 {
		_, _ = SessionManager.Delete(ctx, sessionID)
		return types.ErrEntryNotFound
	}

	sessionData.LastSeenAt = now
	if err := redis.Cache[modeliamsession.Session]().Set(ctx, modeliamsession.SessionIDKey(sessionID), sessionData, ttl); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to touch session", err)
	}
	if err := redis.ZAdd(ctx, modeliamsession.SessionLastSeenKey(), float64(now.UnixMilli()), sessionID); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to touch session", err)
	}
	_ = pruneIndex(ctx, modeliamsession.SessionLastSeenKey(), seenIndexCutoff(now))
	return nil
}

// DeleteUserSessionsExceptCurrent deletes all indexed sessions of a user except the current session.
// Missing session records are treated as stale index entries and cleaned up
// from the user's ZSET so the operation remains idempotent.
func DeleteUserSessionsExceptCurrent(ctx context.Context, userID, currentSessionID string) error {
	if userID == "" {
		return nil
	}

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
				_ = modeliamsession.RemoveSessionIndexes(ctx, userID, sessionID)
				continue
			}
			return service.NewErrorWithCause(http.StatusInternalServerError, "failed to delete session", err)
		}
	}

	return nil
}

// DeleteUserSessions deletes all indexed sessions of a user.
// Missing session records are treated as stale index entries and cleaned up
// from the user's ZSET so the operation remains idempotent.
func DeleteUserSessions(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	modeliamsession.InvalidateUserStateCache(ctx, userID)

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
				_ = modeliamsession.RemoveSessionIndexes(ctx, userID, sessionID)
				continue
			}
			return service.NewErrorWithCause(http.StatusInternalServerError, "failed to delete session", err)
		}
	}

	if err = redis.Del(ctx, modeliamsession.SessionUserKey(userID)); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to delete user session index", err)
	}
	return nil
}
