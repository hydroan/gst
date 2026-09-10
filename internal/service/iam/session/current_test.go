package serviceiamsession_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

func TestCurrentSessionUsesRequestCache(t *testing.T) {
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

func TestCurrentSessionIgnoresMismatchedRequestCache(t *testing.T) {
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
