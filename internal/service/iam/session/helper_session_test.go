package serviceiamsession_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

var (
	setupRedisOnce sync.Once
	errSetupRedis  error
)

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

func TestTouchSession(t *testing.T) {
	setupRedis(t)

	t.Run("throttles_stale_concurrent_snapshot", func(t *testing.T) {
		now := time.Now().UTC()
		sessionID := "touch-session"
		session := modeliamsession.Session{
			ID:         sessionID,
			UserID:     "user-1",
			Status:     modeliamsession.SessionStatusActive,
			IssuedAt:   now.Add(-time.Hour),
			LastSeenAt: now.Add(-time.Minute),
			ExpiresAt:  now.Add(time.Hour),
		}
		require.NoError(t, redis.Cache[modeliamsession.Session]().
			WithContext(t.Context()).
			Set(modeliamsession.SessionIDKey(sessionID), session, time.Until(session.ExpiresAt)))

		firstTouchAt := now
		require.NoError(t, serviceiamsession.TouchSession(t.Context(), sessionID, session, firstTouchAt))

		afterFirstTouch, err := redis.Cache[modeliamsession.Session]().
			WithContext(t.Context()).
			Get(modeliamsession.SessionIDKey(sessionID))
		require.NoError(t, err)
		require.True(t, afterFirstTouch.LastSeenAt.Equal(firstTouchAt))

		staleSnapshot := afterFirstTouch
		staleSnapshot.LastSeenAt = session.LastSeenAt
		require.NoError(t, serviceiamsession.TouchSession(t.Context(), sessionID, staleSnapshot, firstTouchAt.Add(time.Second)))

		afterSecondTouch, err := redis.Cache[modeliamsession.Session]().
			WithContext(t.Context()).
			Get(modeliamsession.SessionIDKey(sessionID))
		require.NoError(t, err)
		require.True(t, afterSecondTouch.LastSeenAt.Equal(afterFirstTouch.LastSeenAt))
	})
}

func TestIndexSessionPrunesStaleLastSeenIndex(t *testing.T) {
	setupRedis(t)

	previousExpiration := serviceiamsession.GetSessionExpiration()
	serviceiamsession.SetSessionExpiration(time.Hour)
	t.Cleanup(func() {
		serviceiamsession.SetSessionExpiration(previousExpiration)
	})

	now := time.Now().UTC()
	staleSessionID := "stale-last-seen-session"
	retainedSessionID := "retained-last-seen-session"
	currentSessionID := "current-session"
	require.NoError(t, redis.ZAdd(t.Context(), modeliamsession.SessionLastSeenKey(), float64(now.Add(-2*time.Hour).UnixMilli()), staleSessionID))
	require.NoError(t, redis.ZAdd(t.Context(), modeliamsession.SessionLastSeenKey(), float64(now.Add(-30*time.Minute).UnixMilli()), retainedSessionID))

	session := modeliamsession.Session{
		ID:         currentSessionID,
		UserID:     "user-1",
		Status:     modeliamsession.SessionStatusActive,
		IssuedAt:   now.Add(-time.Minute),
		LastSeenAt: now,
		ExpiresAt:  now.Add(time.Hour),
	}
	require.NoError(t, serviceiamsession.IndexSession(t.Context(), session))

	lastSeenSessionIDs, err := redis.ZRange(t.Context(), modeliamsession.SessionLastSeenKey(), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, lastSeenSessionIDs, staleSessionID)
	require.Contains(t, lastSeenSessionIDs, retainedSessionID)
	require.Contains(t, lastSeenSessionIDs, currentSessionID)
}

func TestSessionManagerCurrentUsesRequestCache(t *testing.T) {
	now := time.Now().UTC()
	sessionID := "cached-session"
	session := modeliamsession.Session{
		ID:        sessionID,
		UserID:    "user-1",
		Status:    modeliamsession.SessionStatusActive,
		IssuedAt:  now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
	}
	ctx := serviceiamsession.WithCurrentSession(t.Context(), sessionID, session)
	serviceCtx := newSessionServiceContext(ctx, t, sessionID)

	gotSessionID, gotSession, err := serviceiamsession.SessionManager.Current(serviceCtx)
	require.NoError(t, err)
	require.Equal(t, sessionID, gotSessionID)
	require.Equal(t, session, gotSession)
}

func TestSessionManagerCurrentIgnoresMismatchedRequestCache(t *testing.T) {
	setupRedis(t)

	now := time.Now().UTC()
	cookieSessionID := "redis-session"
	cookieSession := modeliamsession.Session{
		ID:        cookieSessionID,
		UserID:    "user-1",
		Status:    modeliamsession.SessionStatusActive,
		IssuedAt:  now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, redis.Cache[modeliamsession.Session]().
		WithContext(t.Context()).
		Set(modeliamsession.SessionIDKey(cookieSessionID), cookieSession, time.Until(cookieSession.ExpiresAt)))

	cachedSessionID := "cached-session"
	cachedSession := modeliamsession.Session{
		ID:        cachedSessionID,
		UserID:    "user-2",
		Status:    modeliamsession.SessionStatusActive,
		IssuedAt:  now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
	}
	ctx := serviceiamsession.WithCurrentSession(t.Context(), cachedSessionID, cachedSession)
	serviceCtx := newSessionServiceContext(ctx, t, cookieSessionID)

	gotSessionID, gotSession, err := serviceiamsession.SessionManager.Current(serviceCtx)
	require.NoError(t, err)
	require.Equal(t, cookieSessionID, gotSessionID)
	require.Equal(t, cookieSession, gotSession)
}

func setupRedis(t *testing.T) {
	t.Helper()

	setupRedisOnce.Do(func() {
		t.Setenv(config.REDIS_ENABLE, "true")
		testutil.SetupRandomRedisNamespace()
		if errSetupRedis = config.Init(); errSetupRedis != nil {
			return
		}
		errSetupRedis = redis.Init()
	})
	require.NoError(t, errSetupRedis)
	require.NoError(t, redis.RemovePrefix(context.Background(), modeliamsession.SessionNamespacePrefix))
	t.Cleanup(func() {
		require.NoError(t, redis.RemovePrefix(context.Background(), modeliamsession.SessionNamespacePrefix))
	})
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
