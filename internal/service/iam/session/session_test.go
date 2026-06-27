package serviceiamsession_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/provider/redis"
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
			State:      modeliamsession.SessionStatusActive,
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
