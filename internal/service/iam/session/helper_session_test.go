package serviceiamsession_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{Redis: true})
}

func TestNewSessionIDGeneratesOpaqueRandomToken(t *testing.T) {
	first, err := serviceiamsession.NewSessionID()
	require.NoError(t, err)
	require.Regexp(t, `^[0-9a-f]{64}$`, first)
	require.NotRegexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, first)

	second, err := serviceiamsession.NewSessionID()
	require.NoError(t, err)
	require.Regexp(t, `^[0-9a-f]{64}$`, second)
	require.NotEqual(t, first, second)
}

func TestGetSessionUserStateTTL(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("IAM_SESSION_USER_STATE_TTL", "")

		require.Equal(t, 30*time.Second, serviceiamsession.GetSessionUserStateTTL())
	})

	t.Run("environment_override", func(t *testing.T) {
		t.Setenv("IAM_SESSION_USER_STATE_TTL", "45s")

		require.Equal(t, 45*time.Second, serviceiamsession.GetSessionUserStateTTL())
	})
}

func TestTouchSession(t *testing.T) {
	clearSessions(t)

	// The touch interval is the only throttle on the activity stamp, and it is
	// evaluated against the snapshot the caller already holds, so a touch inside
	// the interval must leave the stored snapshot exactly as it was.
	t.Run("skips_touch_inside_interval", func(t *testing.T) {
		now := time.Now().UTC()
		sessionID := "touch-session-recent"
		lastSeenAt := now.Add(-time.Second)
		session := modeliamsession.Session{
			ID:         sessionID,
			UserID:     "user-1",
			IssuedAt:   now.Add(-time.Hour),
			LastSeenAt: lastSeenAt,
			ExpiresAt:  now.Add(time.Hour),
		}
		require.NoError(t, redis.Cache[modeliamsession.Session]().Set(t.Context(), serviceiamsession.SessionDataKey(sessionID), session, time.Until(session.ExpiresAt)))

		require.NoError(t, serviceiamsession.Store.TouchSession(t.Context(), sessionID, session, now))

		stored, err := redis.Cache[modeliamsession.Session]().Get(t.Context(), serviceiamsession.SessionDataKey(sessionID))
		require.NoError(t, err)
		require.True(t, stored.LastSeenAt.Equal(lastSeenAt))
	})

	t.Run("writes_snapshot_and_index_outside_interval", func(t *testing.T) {
		now := time.Now().UTC()
		sessionID := "touch-session-stale"
		session := modeliamsession.Session{
			ID:         sessionID,
			UserID:     "user-1",
			IssuedAt:   now.Add(-time.Hour),
			LastSeenAt: now.Add(-time.Minute),
			ExpiresAt:  now.Add(time.Hour),
		}
		require.NoError(t, redis.Cache[modeliamsession.Session]().Set(t.Context(), serviceiamsession.SessionDataKey(sessionID), session, time.Until(session.ExpiresAt)))

		require.NoError(t, serviceiamsession.Store.TouchSession(t.Context(), sessionID, session, now))

		stored, err := redis.Cache[modeliamsession.Session]().Get(t.Context(), serviceiamsession.SessionDataKey(sessionID))
		require.NoError(t, err)
		require.True(t, stored.LastSeenAt.Equal(now))
		require.Equal(t, session.ExpiresAt.UnixMilli(), stored.ExpiresAt.UnixMilli())

		// The snapshot and the last-seen index carry the same activity time, which
		// is what lets the online-window query answer from the index alone.
		indexed, err := redis.ZRangeByScore(
			t.Context(),
			serviceiamsession.SessionIndexSeenKey(),
			strconv.FormatInt(now.UnixMilli(), 10),
			strconv.FormatInt(now.UnixMilli(), 10),
		)
		require.NoError(t, err)
		require.Contains(t, indexed, sessionID)
	})
}

func TestIndexSessionSetsIndexTTL(t *testing.T) {
	const lifetime = time.Hour

	previousExpiration := serviceiamsession.GetSessionExpiration()
	serviceiamsession.SetSessionExpiration(lifetime)
	t.Cleanup(func() {
		serviceiamsession.SetSessionExpiration(previousExpiration)
	})

	t.Run("every_index_expires", func(t *testing.T) {
		clearSessions(t)

		now := time.Now().UTC()
		session := modeliamsession.Session{
			ID:         "ttl-session",
			UserID:     "user-ttl",
			IssuedAt:   now,
			LastSeenAt: now,
			ExpiresAt:  now.Add(lifetime),
		}
		require.NoError(t, serviceiamsession.Store.IndexSession(t.Context(), session))

		for _, key := range []string{serviceiamsession.SessionIndexUserKey(session.UserID), serviceiamsession.SessionIndexAllKey()} {
			ttl, err := redis.TTL(t.Context(), key)
			require.NoError(t, err)
			require.LessOrEqual(t, ttl, lifetime, key)
			require.Greater(t, ttl, lifetime-time.Minute, key)
		}

		// The last-seen index is scored by activity rather than by expiry, so it
		// has to outlive the sessions themselves by one touch interval.
		seenTTL, err := redis.TTL(t.Context(), serviceiamsession.SessionIndexSeenKey())
		require.NoError(t, err)
		require.Greater(t, seenTTL, lifetime)
	})

	t.Run("short_lived_session_keeps_shared_index_alive", func(t *testing.T) {
		clearSessions(t)

		now := time.Now().UTC()
		longLived := modeliamsession.Session{
			ID:         "ttl-session-long",
			UserID:     "user-shared-ttl",
			IssuedAt:   now,
			LastSeenAt: now,
			ExpiresAt:  now.Add(lifetime),
		}
		require.NoError(t, serviceiamsession.Store.IndexSession(t.Context(), longLived))

		shortLived := longLived
		shortLived.ID = "ttl-session-short"
		shortLived.ExpiresAt = now.Add(time.Minute)
		require.NoError(t, serviceiamsession.Store.IndexSession(t.Context(), shortLived))

		// A shared index belongs to every member, so the one with the least time
		// left must not decide when the whole index is reclaimed.
		for _, key := range []string{serviceiamsession.SessionIndexUserKey(longLived.UserID), serviceiamsession.SessionIndexAllKey()} {
			ttl, err := redis.TTL(t.Context(), key)
			require.NoError(t, err)
			require.Greater(t, ttl, lifetime-time.Minute, key)
		}
	})
}

func TestIndexSessionPrunesStaleSeenIndex(t *testing.T) {
	clearSessions(t)

	previousExpiration := serviceiamsession.GetSessionExpiration()
	serviceiamsession.SetSessionExpiration(time.Hour)
	t.Cleanup(func() {
		serviceiamsession.SetSessionExpiration(previousExpiration)
	})

	now := time.Now().UTC()
	staleSessionID := "stale-last-seen-session"
	retainedSessionID := "retained-last-seen-session"
	currentSessionID := "current-session"
	require.NoError(t, redis.ZAdd(t.Context(), serviceiamsession.SessionIndexSeenKey(), float64(now.Add(-2*time.Hour).UnixMilli()), staleSessionID))
	require.NoError(t, redis.ZAdd(t.Context(), serviceiamsession.SessionIndexSeenKey(), float64(now.Add(-30*time.Minute).UnixMilli()), retainedSessionID))

	session := modeliamsession.Session{
		ID:         currentSessionID,
		UserID:     "user-1",
		IssuedAt:   now.Add(-time.Minute),
		LastSeenAt: now,
		ExpiresAt:  now.Add(time.Hour),
	}
	require.NoError(t, serviceiamsession.Store.IndexSession(t.Context(), session))

	seenIndexSessionIDs, err := redis.ZRange(t.Context(), serviceiamsession.SessionIndexSeenKey(), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, seenIndexSessionIDs, staleSessionID)
	require.Contains(t, seenIndexSessionIDs, retainedSessionID)
	require.Contains(t, seenIndexSessionIDs, currentSessionID)
}

func TestSessionManagerCurrentUsesRequestCache(t *testing.T) {
	now := time.Now().UTC()
	sessionID := "cached-session"
	session := modeliamsession.Session{
		ID:        sessionID,
		UserID:    "user-1",
		IssuedAt:  now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
	}
	ctx := serviceiamsession.WithCurrentSession(t.Context(), sessionID, session)
	serviceCtx := newSessionServiceContext(ctx, t, sessionID)

	gotSessionID, gotSession, err := serviceiamsession.CurrentSession(serviceCtx)
	require.NoError(t, err)
	require.Equal(t, sessionID, gotSessionID)
	require.Equal(t, session, gotSession)
}

func TestSessionManagerCurrentIgnoresMismatchedRequestCache(t *testing.T) {
	clearSessions(t)

	now := time.Now().UTC()
	cookieSessionID := "redis-session"
	cookieSession := modeliamsession.Session{
		ID:        cookieSessionID,
		UserID:    "user-1",
		IssuedAt:  now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, redis.Cache[modeliamsession.Session]().Set(t.Context(), serviceiamsession.SessionDataKey(cookieSessionID), cookieSession, time.Until(cookieSession.ExpiresAt)))

	cachedSessionID := "cached-session"
	cachedSession := modeliamsession.Session{
		ID:        cachedSessionID,
		UserID:    "user-2",
		IssuedAt:  now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
	}
	ctx := serviceiamsession.WithCurrentSession(t.Context(), cachedSessionID, cachedSession)
	serviceCtx := newSessionServiceContext(ctx, t, cookieSessionID)

	gotSessionID, gotSession, err := serviceiamsession.CurrentSession(serviceCtx)
	require.NoError(t, err)
	require.Equal(t, cookieSessionID, gotSessionID)
	require.Equal(t, cookieSession, gotSession)
}

// clearSessions drops the session keys left by earlier tests in this run. The
// redis container is fresh per run, so this is only about keeping the tests in
// this package from seeing each other's sessions.
func clearSessions(t *testing.T) {
	t.Helper()

	// Both namespaces, because the user-state cache is keyed by user and is
	// therefore deliberately outside the session prefix.
	require.NoError(t, redis.RemovePrefix(context.Background(), serviceiamsession.SessionNamespace))
	require.NoError(t, redis.RemovePrefix(context.Background(), serviceiamsession.UserNamespace))
}

func newSessionServiceContext(baseCtx context.Context, t *testing.T, sessionID string) *types.ServiceContext {
	t.Helper()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/iam/session/current", nil).WithContext(baseCtx)
	ginCtx.Request.AddCookie(&http.Cookie{
		Name:  serviceiamsession.SessionCookieName,
		Value: sessionID,
	})

	return types.NewServiceContext(ginCtx, nil, consts.PHASE_GET)
}
