package serviceiamsession

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

// Store is the only path from IAM to Redis.
//
// Every session key, every index, and the user-state cache are reached through
// the methods below, and the key layout they use is private to this file. A
// caller therefore cannot address storage in a way the store does not offer,
// which is what lets the layout change without every call site agreeing to it —
// the previous arrangement, with exported key builders and Redis calls spread
// across a dozen files, could promise neither.
//
// The methods speak in sessions rather than in Redis commands. A caller asks to
// delete a session, not to delete a string and remove three sorted-set members,
// and the invariant that those happen together lives here rather than in each
// caller's memory.
var Store = store{}

type store struct{}

// The Redis key layout, in two namespaces.
//
// Every key names the role it plays before it names what it is keyed by, so a
// prefix scan can address one role at a time and no key is a prefix of another.
//
// The split between the namespaces follows what a key is scoped to rather than
// which code writes it. Everything a session owns is reclaimed by dropping
// sessionNamespace; the user-state cache survives the sessions that read it,
// because it describes the user.
const (
	sessionNamespace          = "iam:session"
	sessionDataNamespace      = sessionNamespace + ":data"
	sessionIndexNamespace     = sessionNamespace + ":index"
	sessionIndexUserNamespace = sessionIndexNamespace + ":user"
	sessionIndexAllNamespace  = sessionIndexNamespace + ":all"
	sessionIndexSeenNamespace = sessionIndexNamespace + ":seen"

	userNamespace      = "iam:user"
	userStateNamespace = userNamespace + ":state"
)

// sessionTouchInterval is how often a live session refreshes its activity
// stamp. It bounds both the write rate of TouchSession and how far a live
// session's LastSeenAt may lag the present.
const sessionTouchInterval = 30 * time.Second

func namespacedKey(namespace, id string) string {
	return fmt.Sprintf("%s:%s", namespace, id)
}

func sessionDataKey(sessionID string) string { return namespacedKey(sessionDataNamespace, sessionID) }

func sessionIndexUserKey(userID string) string {
	return namespacedKey(sessionIndexUserNamespace, userID)
}
func sessionIndexAllKey() string        { return sessionIndexAllNamespace }
func sessionIndexSeenKey() string       { return sessionIndexSeenNamespace }
func userStateKey(userID string) string { return namespacedKey(userStateNamespace, userID) }

// UserState is the mutable user state an authenticated request has to confirm
// before a session may keep going.
//
// It is cached separately from the session snapshot because it belongs to the
// user: one entry answers for every session that user holds, and it outlives
// any of them.
type UserState struct {
	Status             modeliamuser.UserStatus `json:"status"`
	MustChangePassword bool                    `json:"must_change_password"`
}

// ---------- session snapshots ----------

// LoadSession returns the stored snapshot of a session.
func (store) LoadSession(ctx context.Context, sessionID string) (modeliamsession.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return modeliamsession.Session{}, types.ErrEntryNotFound
	}
	return redis.Cache[modeliamsession.Session]().Get(ctx, sessionDataKey(sessionID))
}

// SaveSession writes a session snapshot with the given lifetime.
func (store) SaveSession(ctx context.Context, sessionData modeliamsession.Session, ttl time.Duration) error {
	if sessionData.ID == "" {
		return service.NewError(http.StatusInternalServerError, "session id is required")
	}
	if err := redis.Cache[modeliamsession.Session]().Set(ctx, sessionDataKey(sessionData.ID), sessionData, ttl); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to store session", err)
	}
	return nil
}

// DeleteSession removes a session snapshot and every index member pointing at
// it, and returns the snapshot it deleted.
//
// The snapshot is read before the delete because the indexes are keyed by the
// owner it names; without it the per-user index could not be cleaned and would
// keep a member no snapshot backs.
func (store) DeleteSession(ctx context.Context, sessionID string) (modeliamsession.Session, error) {
	if sessionID == "" {
		return modeliamsession.Session{}, nil
	}
	cache := redis.Cache[modeliamsession.Session]()

	sessionKey := sessionDataKey(sessionID)
	sessionData, err := cache.Get(ctx, sessionKey)
	if err != nil {
		return modeliamsession.Session{}, err
	}
	if err = cache.Delete(ctx, sessionKey); err != nil {
		return sessionData, err
	}
	if err = Store.DropSessionIndexes(ctx, sessionData.UserID, sessionID); err != nil {
		return sessionData, err
	}

	return sessionData, nil
}

// ---------- session indexes ----------

// IndexSession adds a session to every index and refreshes the index lifetimes.
//
// The user and global indexes are scored by ExpiresAt so read paths can drop
// spent members before loading snapshots; the seen index is scored by
// LastSeenAt so an online-window query can answer from the index alone.
//
// Pruning keeps each index's contents honest and the ttl keeps the key itself
// from outliving every session it names. The two are not interchangeable: Redis
// expires whole keys and never individual sorted-set members, so without the
// sweep an index accumulates ids that resolve to nothing, and without the ttl an
// index nobody reads is never reclaimed at all.
//
// Every index is a shared key, so its lifetime is a property of the module and
// not of the session being written: it comes from the configured session
// lifetime rather than from this session's remaining time. Deriving it from the
// member would let a session with little time left cut short a key that older,
// longer-lived members still depend on.
//
// A partial write is undone. An index missing a live session is a session that
// cannot be listed or revoked, which is worse than a login that failed.
func (store) IndexSession(ctx context.Context, sessionData modeliamsession.Session) error {
	if sessionData.UserID == "" || sessionData.ID == "" {
		return nil
	}
	// A login is the one moment cheap enough to sweep the global seen index on;
	// failing to sweep is no reason to fail the login.
	_ = pruneIndex(ctx, sessionIndexSeenKey(), seenIndexCutoff(time.Now()))
	if time.Until(sessionData.ExpiresAt) <= 0 {
		return service.NewError(http.StatusInternalServerError, "session expired")
	}

	score := float64(sessionData.ExpiresAt.UnixMilli())
	userKey := sessionIndexUserKey(sessionData.UserID)
	if err := redis.ZAdd(ctx, userKey, score, sessionData.ID); err != nil {
		return indexSessionError(err)
	}

	undo := func() { _ = Store.DropSessionIndexes(ctx, sessionData.UserID, sessionData.ID) }
	if err := redis.ZAdd(ctx, sessionIndexAllKey(), score, sessionData.ID); err != nil {
		undo()
		return indexSessionError(err)
	}
	if err := redis.ZAdd(ctx, sessionIndexSeenKey(), float64(sessionData.LastSeenAt.UnixMilli()), sessionData.ID); err != nil {
		undo()
		return indexSessionError(err)
	}
	if err := redis.Expire(ctx, userKey, indexRetention()); err != nil {
		undo()
		return indexSessionError(err)
	}
	if err := redis.Expire(ctx, sessionIndexAllKey(), indexRetention()); err != nil {
		undo()
		return indexSessionError(err)
	}
	if err := redis.Expire(ctx, sessionIndexSeenKey(), seenIndexRetention()); err != nil {
		undo()
		return indexSessionError(err)
	}
	return nil
}

func indexSessionError(err error) error {
	return service.NewErrorWithCause(http.StatusInternalServerError, "failed to index session", err)
}

// DropSessionIndexes removes a session id from every index pointing at it.
//
// An empty userID skips the per-user index, for callers holding nothing but a
// member of a global index and no snapshot to name its owner.
func (store) DropSessionIndexes(ctx context.Context, userID, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if userID != "" {
		if err := redis.ZRem(ctx, sessionIndexUserKey(userID), sessionID); err != nil {
			return dropIndexError(err)
		}
	}
	if err := redis.ZRem(ctx, sessionIndexAllKey(), sessionID); err != nil {
		return dropIndexError(err)
	}
	if err := redis.ZRem(ctx, sessionIndexSeenKey(), sessionID); err != nil {
		return dropIndexError(err)
	}
	return nil
}

// DropUserSessionIndexMember removes one id from a user's index without
// touching the global ones.
//
// It exists for a member that is wrong rather than spent: an id indexed under a
// user the snapshot says it does not belong to. The global indexes still name it
// correctly, so they are left alone.
func (store) DropUserSessionIndexMember(ctx context.Context, userID, sessionID string) error {
	if userID == "" || sessionID == "" {
		return nil
	}
	if err := redis.ZRem(ctx, sessionIndexUserKey(userID), sessionID); err != nil {
		return dropIndexError(err)
	}
	return nil
}

func dropIndexError(err error) error {
	return service.NewErrorWithCause(http.StatusInternalServerError, "failed to remove session index", err)
}

// ListUserSessionIDs returns the indexed session ids of a user, spent members
// swept first.
func (store) ListUserSessionIDs(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return make([]string, 0), nil
	}
	userKey := sessionIndexUserKey(userID)
	if err := pruneIndex(ctx, userKey, time.Now()); err != nil {
		return nil, err
	}
	sessionIDs, err := redis.ZRange(ctx, userKey, 0, -1)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list user sessions", err)
	}
	return sessionIDs, nil
}

// ListAllSessionIDs returns every indexed session id, spent members swept first.
func (store) ListAllSessionIDs(ctx context.Context) ([]string, error) {
	if err := pruneIndex(ctx, sessionIndexAllKey(), time.Now()); err != nil {
		return nil, err
	}
	sessionIDs, err := redis.ZRange(ctx, sessionIndexAllKey(), 0, -1)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list sessions", err)
	}
	return sessionIDs, nil
}

// ListSeenSessionIDs returns the session ids last active at or after since.
//
// These are candidates, not answers: the seen index can outlive the snapshots
// it names, so callers still load and validate each one.
func (store) ListSeenSessionIDs(ctx context.Context, since time.Time) ([]string, error) {
	if since.IsZero() {
		return make([]string, 0), nil
	}
	// Sweeping is hygiene for a query that already filters by score, so its
	// failure is not this caller's answer to give.
	_ = pruneIndex(ctx, sessionIndexSeenKey(), seenIndexCutoff(time.Now()))
	sessionIDs, err := redis.ZRangeByScore(
		ctx,
		sessionIndexSeenKey(),
		strconv.FormatInt(since.UnixMilli(), 10),
		"+inf",
	)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to list online sessions", err)
	}
	return sessionIDs, nil
}

// pruneIndex removes members scored before cutoff from an index.
//
// The cutoff is what the caller's index means by stale, because the two index
// families are scored differently: the user and global indexes carry ExpiresAt,
// so anything scored before now is spent; the seen index carries LastSeenAt, so
// staleness is bounded by seenIndexCutoff instead.
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

// seenIndexRetention is the same span for the seen index, which is scored by
// activity rather than by expiry.
//
// A live session's LastSeenAt lags the present by at most one touch interval,
// because that is how often it is refreshed, so the seen index stays meaningful
// for one interval longer than the session lifetime itself.
func seenIndexRetention() time.Duration {
	return GetSessionExpiration() + sessionTouchInterval
}

// seenIndexCutoff returns the activity timestamp before which a member of the
// seen index cannot belong to a live session. It is the member-level
// counterpart of seenIndexRetention, and both read the same span so a member and
// the key holding it cannot disagree about staleness.
func seenIndexCutoff(now time.Time) time.Time {
	return now.Add(-seenIndexRetention())
}

// ---------- activity ----------

// TouchSession refreshes LastSeenAt for a session at most once per touch interval.
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
func (store) TouchSession(ctx context.Context, sessionID string, sessionData modeliamsession.Session, now time.Time) error {
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
		_, _ = Store.DeleteSession(ctx, sessionID)
		return types.ErrEntryNotFound
	}

	sessionData.LastSeenAt = now
	if err := redis.Cache[modeliamsession.Session]().Set(ctx, sessionDataKey(sessionID), sessionData, ttl); err != nil {
		return touchSessionError(err)
	}
	if err := redis.ZAdd(ctx, sessionIndexSeenKey(), float64(now.UnixMilli()), sessionID); err != nil {
		return touchSessionError(err)
	}
	_ = pruneIndex(ctx, sessionIndexSeenKey(), seenIndexCutoff(now))
	return nil
}

func touchSessionError(err error) error {
	return service.NewErrorWithCause(http.StatusInternalServerError, "failed to touch session", err)
}

// ---------- bulk revocation ----------

// DeleteUserSessions revokes every session a user holds and drops the cached
// user state along with them.
//
// A snapshot the index names but Redis no longer has is a stale member, not a
// failure: it is swept and the remaining sessions are still deleted, which is
// what keeps the operation idempotent for a caller retrying after a partial run.
func (store) DeleteUserSessions(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	Store.DropUserState(ctx, userID)

	if err := deleteUserSessions(ctx, userID, ""); err != nil {
		return err
	}
	if err := redis.Del(ctx, sessionIndexUserKey(userID)); err != nil {
		return service.NewErrorWithCause(http.StatusInternalServerError, "failed to delete user session index", err)
	}
	return nil
}

// DeleteUserSessionsExcept revokes every session a user holds but one.
//
// The kept session's index member is why the index key survives here while
// DeleteUserSessions drops it outright.
func (store) DeleteUserSessionsExcept(ctx context.Context, userID, keepSessionID string) error {
	if userID == "" {
		return nil
	}
	return deleteUserSessions(ctx, userID, keepSessionID)
}

func deleteUserSessions(ctx context.Context, userID, keepSessionID string) error {
	sessionIDs, err := Store.ListUserSessionIDs(ctx, userID)
	if err != nil {
		return err
	}

	for i := range sessionIDs {
		sessionID := sessionIDs[i]
		if sessionID == "" || sessionID == keepSessionID {
			continue
		}
		if _, err = Store.DeleteSession(ctx, sessionID); err != nil {
			if errors.Is(err, types.ErrEntryNotFound) {
				_ = Store.DropSessionIndexes(ctx, userID, sessionID)
				continue
			}
			return service.NewErrorWithCause(http.StatusInternalServerError, "failed to delete session", err)
		}
	}
	return nil
}

// ---------- user state ----------

// LoadUserState returns the cached user state, reporting whether it was there.
//
// A cache that cannot be read is reported as a miss rather than as an error:
// the caller's fallback is to read the database, which is the answer the cache
// was standing in for anyway.
func (store) LoadUserState(ctx context.Context, userID string) (UserState, bool) {
	state, err := redis.Cache[UserState]().Get(ctx, userStateKey(userID))
	if err == nil {
		return state, true
	}
	if !errors.Is(err, types.ErrEntryNotFound) {
		logSessionStoreWarning("failed to load iam user state cache", userID, err)
	}
	return UserState{}, false
}

// SaveUserState caches the user state for the configured ttl.
//
// A cache that refuses the write is not an error the caller has to answer for:
// the state it was given is already the truth, and the next request pays for a
// database read instead.
func (store) SaveUserState(ctx context.Context, userID string, state UserState) {
	if err := redis.Cache[UserState]().Set(ctx, userStateKey(userID), state, GetSessionUserStateTTL()); err != nil {
		logSessionStoreWarning("failed to cache iam user state", userID, err)
	}
}

// DropUserState invalidates the cached user state of a user.
//
// Anything that changes a user's status, credentials, or existence has to call
// this, or the change only takes effect once the entry expires on its own.
// Best-effort by design: callers run it after a change that already happened,
// so a Redis failure must not fail their request, and the ttl is the backstop.
func (store) DropUserState(ctx context.Context, userID string) {
	if userID == "" {
		return
	}
	_ = redis.Del(ctx, userStateKey(userID))
}

// ---------- maintenance ----------

// Purge drops every key IAM owns.
//
// It exists for tests that need a store with nothing in it. Both namespaces are
// dropped, because the user-state cache is deliberately outside the session one.
func (store) Purge(ctx context.Context) error {
	if err := redis.RemovePrefix(ctx, sessionNamespace); err != nil {
		return err
	}
	return redis.RemovePrefix(ctx, userNamespace)
}

// logSessionStoreWarning reports a storage failure the caller is not expected
// to act on, which is why it is logged here rather than returned.
func logSessionStoreWarning(msg, userID string, err error) {
	zap.S().Warnw(msg, "user_id", userID, "error", err)
}
