package iam_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/internal/requestctx"
	internalresponse "github.com/hydroan/gst/internal/response"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

const (
	sessionsPath      = "/api/iam/sessions"
	adminSessionsPath = "/api/iam/admin/sessions"

	// requestMetadataProbeGroupRoute is the probe registered behind the session
	// middleware; see registerRequestMetadataProbe. The authenticated group
	// carries the API path prefix, so requestMetadataProbeRoute is the pattern
	// the middleware reports and requestMetadataProbePath the concrete path.
	requestMetadataProbeGroupRoute = "/probe/request-metadata/:id"
	requestMetadataProbeRoute      = router.APIPathPrefix + requestMetadataProbeGroupRoute
	requestMetadataProbePath       = router.APIPathPrefix + "/probe/request-metadata/item-1"
)

func adminUserSessionsPath(userID string) string {
	return fmt.Sprintf("/api/iam/admin/users/%s/sessions", userID)
}

type sessionTestAccount struct {
	UserID   string
	Username string
	Password string
}

func TestCurrentSessionGet(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("get_current_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)

		cli := sessionClient(t, sessionID)

		rsp, err := cli.Get[iam.CurrentGetRsp](currentPath)
		require.NoError(t, err)

		require.False(t, rsp.ServerTime.IsZero())
		require.NotEmpty(t, rsp.Principal.UserID)
		require.Equal(t, account.Username, rsp.Principal.Username)
		require.False(t, rsp.Principal.MustChangePassword)
		require.Equal(t, modeliamsession.SessionStatusActive, rsp.Session.Status)
		require.False(t, rsp.Session.IssuedAt.IsZero())
		require.False(t, rsp.Session.LastSeenAt.IsZero())
		require.False(t, rsp.Session.ExpiresAt.IsZero())
		require.Positive(t, rsp.Session.ExpiresInSeconds)
		require.True(t, rsp.Session.ExpiresAt.After(rsp.ServerTime))
	})

	t.Run("touches_stale_current_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)

		staleLastSeenAt := time.Now().Add(-time.Minute).UTC()
		session := setSessionLastSeenAt(t, sessionID, staleLastSeenAt)

		cli := sessionClient(t, sessionID)

		rsp, err := cli.Get[iam.CurrentGetRsp](currentPath)
		require.NoError(t, err)

		require.Equal(t, modeliamsession.SessionStatusActive, rsp.Session.Status)
		require.False(t, rsp.Session.LastSeenAt.IsZero())

		after := loadStoredSession(t, sessionID)
		require.Equal(t, session.ExpiresAt, after.ExpiresAt)
		require.True(t, after.LastSeenAt.After(staleLastSeenAt), "expected current session request to touch last_seen_at")
	})

	t.Run("skips_recent_current_session_touch", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)

		recentLastSeenAt := time.Now().Add(-time.Second).UTC()
		session := setSessionLastSeenAt(t, sessionID, recentLastSeenAt)

		cli := sessionClient(t, sessionID)

		rsp, err := cli.Get[iam.CurrentGetRsp](currentPath)
		require.NoError(t, err)
		require.Equal(t, modeliamsession.SessionStatusActive, rsp.Session.Status)
		require.False(t, rsp.Session.LastSeenAt.IsZero())

		after := loadStoredSession(t, sessionID)
		require.Equal(t, session.ExpiresAt, after.ExpiresAt)
		require.Equal(t, recentLastSeenAt, after.LastSeenAt)
	})

	t.Run("returns_current_session_tenant", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		tenantID := "tenant_current_session"
		setSessionTenantID(t, sessionID, tenantID)

		cli := sessionClient(t, sessionID)

		rsp, err := cli.Get[iam.CurrentGetRsp](currentPath)
		require.NoError(t, err)

		require.Equal(t, tenantID, rsp.Session.TenantID)
	})

	t.Run("reject_session_when_stored_snapshot_is_not_active", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		sessionKey := modeliamsession.SessionIDKey(sessionID)

		session, err := redis.Cache[modeliamsession.Session]().Get(t.Context(), sessionKey)
		require.NoError(t, err)
		session.Status = modeliamsession.SessionStatusRevoked
		require.NoError(t, redis.Cache[modeliamsession.Session]().Set(t.Context(), sessionKey, session, time.Hour))

		cli := sessionClient(t, sessionID)

		_, err = cli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})

	t.Run("reject_session_when_stored_snapshot_is_expired", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		sessionKey := modeliamsession.SessionIDKey(sessionID)

		session, err := redis.Cache[modeliamsession.Session]().Get(t.Context(), sessionKey)
		require.NoError(t, err)
		session.ExpiresAt = time.Now().Add(-time.Minute)
		require.NoError(t, redis.Cache[modeliamsession.Session]().Set(t.Context(), sessionKey, session, time.Hour))

		cli := sessionClient(t, sessionID)

		_, err = cli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})
}

func TestCurrentSessionDelete(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("delete_current_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		requireUserSessionContains(t, account.UserID, sessionID)

		cli := sessionClient(t, sessionID)

		rsp, err := cli.Delete[iam.CurrentDeleteRsp](currentPath, nil)
		require.NoError(t, err)
		require.Equal(t, iam.CurrentDeleteRsp{}, *rsp)

		requireSessionNotFound(t, sessionID)
		requireUserSessionNotContains(t, account.UserID, sessionID)

		_, err = cli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})
}

func TestSessionGet(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("get_current_user_session_detail", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		otherSessionID := loginSession(t, account.Username, account.Password)
		tenantID := "tenant_session_get"
		setSessionTenantID(t, otherSessionID, tenantID)

		cli := sessionClient(t, currentSessionID)

		rsp, err := cli.Get[iam.SessionGetRsp](sessionsPath + "/" + otherSessionID)
		require.NoError(t, err)
		require.Equal(t, otherSessionID, rsp.Session.ID)
		require.Equal(t, tenantID, rsp.Session.TenantID)
		require.False(t, rsp.Session.IsCurrent)
	})

	t.Run("get_current_session_detail", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)

		cli := sessionClient(t, currentSessionID)

		rsp, err := cli.Get[iam.SessionGetRsp](sessionsPath + "/" + currentSessionID)
		require.NoError(t, err)
		require.Equal(t, currentSessionID, rsp.Session.ID)
		require.True(t, rsp.Session.IsCurrent)
	})

	t.Run("forbidden_when_getting_other_user_session", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		victimSessionID := loginSession(t, victim.Username, victim.Password)

		cli := sessionClient(t, attackerSessionID)

		_, err := cli.Get[iam.SessionGetRsp](sessionsPath + "/" + victimSessionID)
		testutil.RequireError(t, err, http.StatusForbidden)
	})

	t.Run("not_found_when_session_missing", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)

		cli := sessionClient(t, currentSessionID)

		_, err := cli.Get[iam.SessionGetRsp](sessionsPath + "/missing-session-id")
		testutil.RequireError(t, err, http.StatusNotFound)
	})
}

func TestSessionList(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("list_current_user_sessions", func(t *testing.T) {
		account := newSessionTestAccount(t)
		otherSessionID := loginSession(t, account.Username, account.Password)
		currentSessionID := loginSession(t, account.Username, account.Password)

		cli := sessionClient(t, currentSessionID)

		list, err := cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		require.NoError(t, err)

		require.Len(t, list.Items, 2)
		require.Equal(t, 2, list.Total)

		sessionMap := make(map[string]iam.SessionView, len(list.Items))
		for i := range list.Items {
			sessionMap[list.Items[i].ID] = list.Items[i]
		}

		require.Contains(t, sessionMap, currentSessionID)
		require.Contains(t, sessionMap, otherSessionID)
		require.True(t, sessionMap[currentSessionID].IsCurrent)
		require.False(t, sessionMap[otherSessionID].IsCurrent)
	})

	t.Run("ignore_request_body_on_list", func(t *testing.T) {
		// List handles an HTTP GET request whose body carries no semantics;
		// the controller must not bind (or reject) whatever body a client
		// happens to send.
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+sessionsPath, strings.NewReader("{invalid json"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		// Match the framework client's User-Agent: the session is bound to
		// the browser fingerprint captured at login.
		req.Header.Set("User-Agent", consts.FrameworkName)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("reject_when_user_disabled_after_session_created", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		sessionSetUserStatus(t, account.Username, modeliamuser.UserStatusInactive)

		cli := sessionClient(t, sessionID)

		_, err := cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		testutil.RequireError(t, err, http.StatusForbidden, "account disabled")
	})

	t.Run("reject_when_user_locked_after_session_created", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		sessionSetUserStatus(t, account.Username, modeliamuser.UserStatusLocked)

		cli := sessionClient(t, sessionID)

		_, err := cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		testutil.RequireError(t, err, http.StatusForbidden, "account locked")
	})

	t.Run("prune_invalid_indexed_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		expiredSessionID := loginSession(t, account.Username, account.Password)
		currentSessionID := loginSession(t, account.Username, account.Password)

		sessionKey := modeliamsession.SessionIDKey(expiredSessionID)
		session, err := redis.Cache[modeliamsession.Session]().Get(t.Context(), sessionKey)
		require.NoError(t, err)
		session.ExpiresAt = time.Now().Add(-time.Minute)
		require.NoError(t, redis.Cache[modeliamsession.Session]().Set(t.Context(), sessionKey, session, time.Hour))
		requireUserSessionContains(t, account.UserID, expiredSessionID)
		requireAllSessionContains(t, expiredSessionID)

		cli := sessionClient(t, currentSessionID)

		list, err := cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		require.NoError(t, err)

		require.Len(t, list.Items, 1)
		require.Equal(t, 1, list.Total)
		require.Equal(t, currentSessionID, list.Items[0].ID)
		require.True(t, list.Items[0].IsCurrent)

		requireUserSessionNotContains(t, account.UserID, expiredSessionID)
		requireAllSessionNotContains(t, expiredSessionID)
	})
}

func TestSessionUserStateRefresh(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("returns_error_when_db_refresh_fails", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		session := loadStoredSession(t, sessionID)
		modeliamsession.InvalidateUserStateCache(context.Background(), account.UserID)

		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := serviceiamsession.ValidateSessionUserState(canceledCtx, session)
		require.Error(t, err)

		var serviceErr *service.Error
		require.True(t, errors.As(err, &serviceErr))
		require.Equal(t, http.StatusInternalServerError, serviceErr.Status())
		require.Contains(t, err.Error(), "failed to refresh session user state")
	})
}

// TestInvalidateUserSessions covers the revocation entry point user lifecycle
// operations reach for. Those operations live wherever a project models its
// users, which is regularly outside the IAM service package, so this asserts the
// guarantee they depend on: once revocation returns, the cookie is dead, without
// waiting for the user-state cache to expire.
func TestInvalidateUserSessions(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("revokes_every_session_and_rejects_the_stale_cookie", func(t *testing.T) {
		account := newSessionTestAccount(t)
		firstSessionID := loginSession(t, account.Username, account.Password)
		secondSessionID := loginSession(t, account.Username, account.Password)

		// Spend the session once so the user-state cache is warm. A warm cache is
		// exactly what lets a revoked user keep authenticating when only the
		// database row changed.
		_, err := sessionClient(t, firstSessionID).Get[iam.CurrentGetRsp](currentPath)
		require.NoError(t, err)

		modeliamsession.InvalidateUserSessions(context.Background(), account.UserID)

		requireSessionNotFound(t, firstSessionID)
		requireSessionNotFound(t, secondSessionID)
		requireUserSessionNotContains(t, account.UserID, firstSessionID)
		requireUserSessionNotContains(t, account.UserID, secondSessionID)
		requireAllSessionNotContains(t, firstSessionID)
		requireAllSessionNotContains(t, secondSessionID)
		requireLastSeenSessionNotContains(t, firstSessionID)
		requireLastSeenSessionNotContains(t, secondSessionID)
		requireUserStateCacheCleared(t, account.UserID)

		_, err = sessionClient(t, firstSessionID).Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})

	t.Run("leaves_other_users_signed_in", func(t *testing.T) {
		target := newSessionTestAccount(t)
		targetSessionID := loginSession(t, target.Username, target.Password)

		bystander := newSessionTestAccount(t)
		bystanderSessionID := loginSession(t, bystander.Username, bystander.Password)

		modeliamsession.InvalidateUserSessions(context.Background(), target.UserID)

		requireSessionNotFound(t, targetSessionID)
		requireUserSessionContains(t, bystander.UserID, bystanderSessionID)
		requireAllSessionContains(t, bystanderSessionID)

		_, err := sessionClient(t, bystanderSessionID).Get[iam.CurrentGetRsp](currentPath)
		require.NoError(t, err)
	})
}

// TestSessionRejectionsAnswerInTheEnvelope covers the shape of a refusal from
// the session middleware, which every request in a deployment can receive.
//
// Two properties are asserted, and each was broken once. These refusals used to
// answer with a bare {"error": ...}: a client reading the documented envelope
// found no code and no trace id, so a rejection could not be told apart from a
// malformed response, and the one identifier tying it to the server's logs was
// absent. And the message came from whatever layer failed — a cookie naming a
// session the cache no longer holds was answered "cache entry not found",
// handing the storage's internal vocabulary to whoever presented an invalid
// cookie. What a caller can act on is that the cookie is not usable; which
// layer noticed is the deployment's business.
func TestSessionRejectionsAnswerInTheEnvelope(t *testing.T) {
	clearSessionsAfterTest(t)

	requireEnvelopeRejection := func(t *testing.T, err error, wantMsg string) {
		t.Helper()

		respErr := testutil.RequireError(t, err, http.StatusUnauthorized, wantMsg)
		var envelope struct {
			Code    *int             `json:"code"`
			Msg     string           `json:"msg"`
			Data    *json.RawMessage `json:"data"`
			TraceID *string          `json:"trace_id"`
		}
		require.NoError(t, json.Unmarshal(respErr.Body, &envelope), "response body: %s", respErr.Body)
		require.NotNil(t, envelope.Code, "a rejection has to carry a code, like every other response")
		require.NotNil(t, envelope.TraceID, "a rejection has to carry the trace that explains it")
		require.Equal(t, wantMsg, envelope.Msg)
	}

	t.Run("no cookie", func(t *testing.T) {
		cli, err := client.New(baseURL)
		require.NoError(t, err)

		_, err = cli.Get[iam.CurrentGetRsp](currentPath)
		requireEnvelopeRejection(t, err, "no session")
	})

	t.Run("a cookie naming no session", func(t *testing.T) {
		// The storage answers "entry not found"; the client is told the one
		// thing it can act on.
		cli := sessionClient(t, "0000000000000000000000000000000000000000000000000000000000000000")
		_, err := cli.Get[iam.CurrentGetRsp](currentPath)
		requireEnvelopeRejection(t, err, "session invalid")
	})

	t.Run("a revoked session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		_, err := sessionClient(t, sessionID).Get[iam.CurrentGetRsp](currentPath)
		require.NoError(t, err)

		modeliamsession.InvalidateUserSessions(context.Background(), account.UserID)

		_, err = sessionClient(t, sessionID).Get[iam.CurrentGetRsp](currentPath)
		requireEnvelopeRejection(t, err, "session invalid")
	})

	t.Run("a cookie presented from another browser", func(t *testing.T) {
		// A session is bound to the user agent that opened it, and the binding
		// is checked one component at a time. Naming the component that failed
		// would let the bearer of a stolen cookie recover the rest of it one
		// request at a time, so this answers as every other unusable cookie.
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(
			baseURL,
			client.WithUserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
			client.WithCookie(&http.Cookie{Name: "session_id", Value: sessionID}),
		)
		require.NoError(t, err)

		_, err = cli.Get[iam.CurrentGetRsp](currentPath)
		requireEnvelopeRejection(t, err, "session invalid")
	})
}

// TestSessionMiddlewareCarriesRequestMetadata pins that the context the session
// middleware works on, and hands downstream, carries the request metadata. The
// middleware loads, validates and touches the session before any handler runs,
// and each of those steps reaches storage on that context; without the metadata
// their statements are logged with no route and cannot be aggregated per
// endpoint.
//
// Only the fields the request answers for itself are asserted. The identity is
// not among them: the middleware publishes that once every check has passed, so
// the context it works on names a route and a path but nobody yet.
func TestSessionMiddlewareCarriesRequestMetadata(t *testing.T) {
	clearSessionsAfterTest(t)

	account := newSessionTestAccount(t)
	sessionID := loginSession(t, account.Username, account.Password)

	rsp, err := sessionClient(t, sessionID).Get[requestMetadataProbeRsp](
		requestMetadataProbePath, client.WithQuery("limit", 10),
	)
	require.NoError(t, err)

	require.Equal(t, requestMetadataProbeRoute, rsp.Route)
	require.Equal(t, requestMetadataProbePath, rsp.Path)
	require.Equal(t, "limit=10", rsp.RawQuery)
}

func TestAdminSessionList(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("list_all_sessions_grouped_by_user", func(t *testing.T) {
		adminAccount := rootSessionTestAccount()
		adminSessionID := sessionLoginRoot(t)

		firstUser := newSessionTestAccount(t)
		firstUserSessionID1 := loginSession(t, firstUser.Username, firstUser.Password)
		firstUserSessionID2 := loginSession(t, firstUser.Username, firstUser.Password)

		secondUser := newSessionTestAccount(t)
		secondUserSessionID := loginSession(t, secondUser.Username, secondUser.Password)

		requireAllSessionContains(t, adminSessionID)
		requireAllSessionContains(t, firstUserSessionID1)
		requireAllSessionContains(t, firstUserSessionID2)
		requireAllSessionContains(t, secondUserSessionID)

		cli := sessionClient(t, adminSessionID)

		rsp, err := cli.Get[iam.AdminSessionListRsp](adminSessionsPath)
		require.NoError(t, err)

		require.GreaterOrEqual(t, rsp.Total, 3)
		require.GreaterOrEqual(t, rsp.SessionTotal, 4)

		userMap := make(map[string]iam.AdminSessionOwnerView, len(rsp.Items))
		for i := range rsp.Items {
			userMap[rsp.Items[i].Username] = rsp.Items[i]
		}

		require.Contains(t, userMap, adminAccount.Username)
		require.Contains(t, userMap, firstUser.Username)
		require.Contains(t, userMap, secondUser.Username)

		require.Len(t, userMap[adminAccount.Username].Sessions, 1)
		require.Equal(t, adminSessionID, userMap[adminAccount.Username].Sessions[0].ID)
		require.False(t, userMap[adminAccount.Username].Sessions[0].IsCurrent)

		firstUserSessionIDs := make(map[string]struct{}, len(userMap[firstUser.Username].Sessions))
		for i := range userMap[firstUser.Username].Sessions {
			firstUserSessionIDs[userMap[firstUser.Username].Sessions[i].ID] = struct{}{}
			require.False(t, userMap[firstUser.Username].Sessions[i].IsCurrent)
		}
		require.Len(t, userMap[firstUser.Username].Sessions, 2)
		_, ok := firstUserSessionIDs[firstUserSessionID1]
		require.True(t, ok)
		_, ok = firstUserSessionIDs[firstUserSessionID2]
		require.True(t, ok)

		require.Len(t, userMap[secondUser.Username].Sessions, 1)
		require.Equal(t, secondUserSessionID, userMap[secondUser.Username].Sessions[0].ID)
		require.False(t, userMap[secondUser.Username].Sessions[0].IsCurrent)
	})

	t.Run("list_online_sessions_within_window", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		targetAccount := newSessionTestAccount(t)
		recentSessionID := loginSession(t, targetAccount.Username, targetAccount.Password)
		staleSessionID := loginSession(t, targetAccount.Username, targetAccount.Password)

		now := time.Now().UTC()
		setSessionLastSeenAt(t, recentSessionID, now.Add(-time.Minute))
		setSessionLastSeenAt(t, staleSessionID, now.Add(-10*time.Minute))

		cli := sessionClient(t, adminSessionID)

		rsp, err := cli.Get[iam.AdminSessionListRsp](adminSessionsPath,
			client.WithQuery("online_within", "5m"))
		require.NoError(t, err)

		userMap := make(map[string]iam.AdminSessionOwnerView, len(rsp.Items))
		for i := range rsp.Items {
			userMap[rsp.Items[i].Username] = rsp.Items[i]
		}

		require.Contains(t, userMap, targetAccount.Username)
		require.Len(t, userMap[targetAccount.Username].Sessions, 1)
		require.Equal(t, recentSessionID, userMap[targetAccount.Username].Sessions[0].ID)
	})

	t.Run("reject_invalid_online_within", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		cli := sessionClient(t, adminSessionID)

		_, err := cli.Get[iam.AdminSessionListRsp](adminSessionsPath,
			client.WithQuery("online_within", "bad"))
		testutil.RequireError(t, err, http.StatusBadRequest)
	})

	t.Run("forbidden_for_regular_user", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)

		cli := sessionClient(t, sessionID)

		_, err := cli.Get[iam.AdminSessionListRsp](adminSessionsPath)
		testutil.RequireError(t, err, http.StatusForbidden)
	})

	t.Run("forbidden_for_inactive_root", func(t *testing.T) {
		adminAccount := rootSessionTestAccount()
		adminSessionID := sessionLoginRoot(t)
		sessionSetUserStatus(t, adminAccount.Username, modeliamuser.UserStatusInactive)
		t.Cleanup(func() {
			sessionSetUserStatus(t, adminAccount.Username, modeliamuser.UserStatusActive)
		})

		cli := sessionClient(t, adminSessionID)

		_, err := cli.Get[iam.AdminSessionListRsp](adminSessionsPath)
		testutil.RequireError(t, err, http.StatusForbidden, "account disabled")
	})

	t.Run("forbidden_for_locked_root", func(t *testing.T) {
		adminAccount := rootSessionTestAccount()
		adminSessionID := sessionLoginRoot(t)
		sessionSetUserStatus(t, adminAccount.Username, modeliamuser.UserStatusLocked)
		t.Cleanup(func() {
			sessionSetUserStatus(t, adminAccount.Username, modeliamuser.UserStatusActive)
		})

		cli := sessionClient(t, adminSessionID)

		_, err := cli.Get[iam.AdminSessionListRsp](adminSessionsPath)
		testutil.RequireError(t, err, http.StatusForbidden, "account locked")
	})
}

func TestAdminSessionGet(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("get_other_user_session_detail", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		targetAccount := newSessionTestAccount(t)
		targetSessionID := loginSession(t, targetAccount.Username, targetAccount.Password)

		cli := sessionClient(t, adminSessionID)

		rsp, err := cli.Get[modeliamsession.AdminSessionGetRsp](adminSessionsPath + "/" + targetSessionID)
		require.NoError(t, err)
		require.Equal(t, targetSessionID, rsp.Session.ID)
		require.Equal(t, modeliamsession.SessionStatusActive, rsp.Session.Status)
		require.False(t, rsp.Session.IsCurrent)
		require.NotEmpty(t, rsp.Session.ClientIP)
		require.NotEmpty(t, rsp.Session.BrowserName)
	})

	t.Run("forbidden_for_regular_user", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		victimSessionID := loginSession(t, victim.Username, victim.Password)

		cli := sessionClient(t, attackerSessionID)

		_, err := cli.Get[modeliamsession.AdminSessionGetRsp](adminSessionsPath + "/" + victimSessionID)
		testutil.RequireError(t, err, http.StatusForbidden)
	})

	t.Run("not_found_when_session_missing", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		cli := sessionClient(t, adminSessionID)

		_, err := cli.Get[modeliamsession.AdminSessionGetRsp](adminSessionsPath + "/missing-session-id")
		testutil.RequireError(t, err, http.StatusNotFound)
	})
}

func TestAdminSessionDelete(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("delete_other_user_session", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		targetAccount := newSessionTestAccount(t)
		targetSessionID := loginSession(t, targetAccount.Username, targetAccount.Password)

		requireUserSessionContains(t, targetAccount.UserID, targetSessionID)
		requireAllSessionContains(t, targetSessionID)

		cli := sessionClient(t, adminSessionID)

		rsp, err := cli.Delete[iam.AdminSessionDeleteRsp](adminSessionsPath+"/"+targetSessionID, nil)
		require.NoError(t, err)
		require.Equal(t, iam.AdminSessionDeleteRsp{}, *rsp)

		requireSessionNotFound(t, targetSessionID)
		requireUserSessionNotContains(t, targetAccount.UserID, targetSessionID)
	})

	t.Run("forbidden_for_regular_user", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		victimSessionID := loginSession(t, victim.Username, victim.Password)
		requireUserSessionContains(t, victim.UserID, victimSessionID)

		cli := sessionClient(t, attackerSessionID)

		_, err := cli.Delete[iam.AdminSessionDeleteRsp](adminSessionsPath+"/"+victimSessionID, nil)
		testutil.RequireError(t, err, http.StatusForbidden)

		requireUserSessionContains(t, victim.UserID, victimSessionID)
	})

	t.Run("not_found_when_session_missing", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		cli := sessionClient(t, adminSessionID)

		_, err := cli.Delete[iam.AdminSessionDeleteRsp](adminSessionsPath+"/missing-session-id", nil)
		testutil.RequireError(t, err, http.StatusNotFound)
	})
}

func TestAdminUserSessionList(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("list_all_sessions_of_target_user", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		targetAccount := newSessionTestAccount(t)
		targetSessionID1 := loginSession(t, targetAccount.Username, targetAccount.Password)
		targetSessionID2 := loginSession(t, targetAccount.Username, targetAccount.Password)

		cli := sessionClient(t, adminSessionID)

		rsp, err := cli.Get[iam.AdminUserSessionListRsp](adminUserSessionsPath(targetAccount.UserID))
		require.NoError(t, err)
		require.Equal(t, targetAccount.UserID, rsp.User.UserID)
		require.Equal(t, targetAccount.Username, rsp.User.Username)
		require.Len(t, rsp.User.Sessions, 2)

		sessionMap := make(map[string]iam.SessionView, len(rsp.User.Sessions))
		for i := range rsp.User.Sessions {
			sessionMap[rsp.User.Sessions[i].ID] = rsp.User.Sessions[i]
			require.False(t, rsp.User.Sessions[i].IsCurrent)
		}

		require.Contains(t, sessionMap, targetSessionID1)
		require.Contains(t, sessionMap, targetSessionID2)
	})

	t.Run("list_online_sessions_within_window", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		targetAccount := newSessionTestAccount(t)
		recentSessionID := loginSession(t, targetAccount.Username, targetAccount.Password)
		staleSessionID := loginSession(t, targetAccount.Username, targetAccount.Password)

		now := time.Now().UTC()
		setSessionLastSeenAt(t, recentSessionID, now.Add(-time.Minute))
		setSessionLastSeenAt(t, staleSessionID, now.Add(-10*time.Minute))

		cli := sessionClient(t, adminSessionID)

		rsp, err := cli.Get[iam.AdminUserSessionListRsp](adminUserSessionsPath(targetAccount.UserID),
			client.WithQuery("online_within", "5m"))
		require.NoError(t, err)
		require.Equal(t, targetAccount.UserID, rsp.User.UserID)
		require.Equal(t, targetAccount.Username, rsp.User.Username)
		require.Len(t, rsp.User.Sessions, 1)
		require.Equal(t, recentSessionID, rsp.User.Sessions[0].ID)
	})

	t.Run("forbidden_for_regular_user", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		_ = loginSession(t, victim.Username, victim.Password)

		cli := sessionClient(t, attackerSessionID)

		_, err := cli.Get[iam.AdminUserSessionListRsp](adminUserSessionsPath(victim.UserID))
		testutil.RequireError(t, err, http.StatusForbidden)
	})

	t.Run("not_found_when_user_missing", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		cli := sessionClient(t, adminSessionID)

		_, err := cli.Get[iam.AdminUserSessionListRsp](adminUserSessionsPath("missing-user-id"))
		testutil.RequireError(t, err, http.StatusNotFound)
	})

	t.Run("list_target_user_with_no_sessions", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		targetAccount := newSessionTestAccount(t)

		cli := sessionClient(t, adminSessionID)

		rsp, err := cli.Get[iam.AdminUserSessionListRsp](adminUserSessionsPath(targetAccount.UserID))
		require.NoError(t, err)
		require.Equal(t, targetAccount.UserID, rsp.User.UserID)
		require.Equal(t, targetAccount.Username, rsp.User.Username)
		require.Empty(t, rsp.User.Sessions)
	})

	t.Run("mark_current_session_when_admin_views_self", func(t *testing.T) {
		adminAccount := rootSessionTestAccount()
		currentAdminSessionID := sessionLoginRoot(t)
		otherAdminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		cli := sessionClient(t, currentAdminSessionID)

		rsp, err := cli.Get[iam.AdminUserSessionListRsp](adminUserSessionsPath(adminAccount.UserID))
		require.NoError(t, err)
		require.Len(t, rsp.User.Sessions, 2)

		sessionMap := make(map[string]iam.SessionView, len(rsp.User.Sessions))
		for i := range rsp.User.Sessions {
			sessionMap[rsp.User.Sessions[i].ID] = rsp.User.Sessions[i]
		}

		require.True(t, sessionMap[currentAdminSessionID].IsCurrent)
		require.False(t, sessionMap[otherAdminSessionID].IsCurrent)
	})
}

func TestAdminUserSessionDelete(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("delete_all_sessions_of_target_user", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		targetAccount := newSessionTestAccount(t)
		targetSessionID1 := loginSession(t, targetAccount.Username, targetAccount.Password)
		targetSessionID2 := loginSession(t, targetAccount.Username, targetAccount.Password)

		requireUserSessionContains(t, targetAccount.UserID, targetSessionID1)
		requireUserSessionContains(t, targetAccount.UserID, targetSessionID2)
		requireAllSessionContains(t, targetSessionID1)
		requireAllSessionContains(t, targetSessionID2)

		cli := sessionClient(t, adminSessionID)

		rsp, err := cli.Delete[iam.AdminUserSessionDeleteRsp](adminUserSessionsPath(targetAccount.UserID), nil)
		require.NoError(t, err)
		require.Equal(t, iam.AdminUserSessionDeleteRsp{}, *rsp)

		requireSessionNotFound(t, targetSessionID1)
		requireSessionNotFound(t, targetSessionID2)
		requireUserSessionNotContains(t, targetAccount.UserID, targetSessionID1)
		requireUserSessionNotContains(t, targetAccount.UserID, targetSessionID2)
		requireAllSessionNotContains(t, targetSessionID1)
		requireAllSessionNotContains(t, targetSessionID2)
	})

	t.Run("forbidden_for_regular_user", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		victimSessionID := loginSession(t, victim.Username, victim.Password)
		requireUserSessionContains(t, victim.UserID, victimSessionID)

		cli := sessionClient(t, attackerSessionID)

		_, err := cli.Delete[iam.AdminUserSessionDeleteRsp](adminUserSessionsPath(victim.UserID), nil)
		testutil.RequireError(t, err, http.StatusForbidden)

		requireUserSessionContains(t, victim.UserID, victimSessionID)
	})

	t.Run("not_found_when_user_missing", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		cli := sessionClient(t, adminSessionID)

		_, err := cli.Delete[iam.AdminUserSessionDeleteRsp](adminUserSessionsPath("missing-user-id"), nil)
		testutil.RequireError(t, err, http.StatusNotFound)
	})

	t.Run("idempotent_when_target_user_has_no_sessions", func(t *testing.T) {
		adminSessionID := sessionLoginRoot(t)

		targetAccount := newSessionTestAccount(t)

		cli := sessionClient(t, adminSessionID)

		rsp, err := cli.Delete[iam.AdminUserSessionDeleteRsp](adminUserSessionsPath(targetAccount.UserID), nil)
		require.NoError(t, err)
		require.Equal(t, iam.AdminUserSessionDeleteRsp{}, *rsp)
	})

	t.Run("delete_all_sessions_of_current_admin", func(t *testing.T) {
		adminAccount := rootSessionTestAccount()
		currentAdminSessionID := sessionLoginRoot(t)
		otherAdminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		requireUserSessionContains(t, adminAccount.UserID, currentAdminSessionID)
		requireUserSessionContains(t, adminAccount.UserID, otherAdminSessionID)

		cli := sessionClient(t, currentAdminSessionID)

		rsp, err := cli.Delete[iam.AdminUserSessionDeleteRsp](adminUserSessionsPath(adminAccount.UserID), nil)
		require.NoError(t, err)
		require.Equal(t, iam.AdminUserSessionDeleteRsp{}, *rsp)

		requireSessionNotFound(t, currentAdminSessionID)
		requireSessionNotFound(t, otherAdminSessionID)
		requireUserSessionNotContains(t, adminAccount.UserID, currentAdminSessionID)
		requireUserSessionNotContains(t, adminAccount.UserID, otherAdminSessionID)
		requireAllSessionNotContains(t, currentAdminSessionID)
		requireAllSessionNotContains(t, otherAdminSessionID)

		currentCli := sessionClient(t, currentAdminSessionID)

		_, err = currentCli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})
}

func TestSessionDelete(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("delete_non_current_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		otherSessionID := loginSession(t, account.Username, account.Password)

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionContains(t, account.UserID, otherSessionID)

		cli := sessionClient(t, currentSessionID)

		rsp, err := cli.Delete[iam.SessionDeleteRsp](sessionsPath+"/"+otherSessionID, nil)
		require.NoError(t, err)
		require.Equal(t, iam.SessionDeleteRsp{}, *rsp)

		list, err := cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		require.NoError(t, err)
		require.Len(t, list.Items, 1)
		require.Equal(t, 1, list.Total)
		require.Equal(t, currentSessionID, list.Items[0].ID)
		require.True(t, list.Items[0].IsCurrent)

		requireSessionNotFound(t, otherSessionID)
		requireUserSessionNotContains(t, account.UserID, otherSessionID)
		requireUserSessionContains(t, account.UserID, currentSessionID)
	})

	t.Run("delete_missing_session_is_idempotent", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		missingSessionID := loginSession(t, account.Username, account.Password)

		cli := sessionClient(t, currentSessionID)

		_, err := cli.Delete[iam.SessionDeleteRsp](sessionsPath+"/"+missingSessionID, nil)
		require.NoError(t, err)

		rsp, err := cli.Delete[iam.SessionDeleteRsp](sessionsPath+"/"+missingSessionID, nil)
		require.NoError(t, err)
		require.Equal(t, iam.SessionDeleteRsp{}, *rsp)

		list, err := cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		require.NoError(t, err)
		require.Len(t, list.Items, 1)
		require.Equal(t, 1, list.Total)
		require.Equal(t, currentSessionID, list.Items[0].ID)
		require.True(t, list.Items[0].IsCurrent)
	})

	t.Run("forbidden_when_deleting_other_user_session", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		victimSessionID := loginSession(t, victim.Username, victim.Password)
		requireUserSessionContains(t, victim.UserID, victimSessionID)

		cli := sessionClient(t, attackerSessionID)

		_, err := cli.Delete[iam.SessionDeleteRsp](sessionsPath+"/"+victimSessionID, nil)
		testutil.RequireError(t, err, http.StatusForbidden)

		requireUserSessionContains(t, victim.UserID, victimSessionID)
	})

	t.Run("delete_current_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		requireUserSessionContains(t, account.UserID, sessionID)

		cli := sessionClient(t, sessionID)

		rsp, err := cli.Delete[iam.SessionDeleteRsp](sessionsPath+"/"+sessionID, nil)
		require.NoError(t, err)
		require.Equal(t, iam.SessionDeleteRsp{}, *rsp)

		requireSessionNotFound(t, sessionID)
		requireUserSessionNotContains(t, account.UserID, sessionID)

		currentCli := sessionClient(t, sessionID)

		_, err = currentCli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})
}

func TestSessionDeleteOthers(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("delete_all_other_sessions", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		otherSessionID1 := loginSession(t, account.Username, account.Password)
		otherSessionID2 := loginSession(t, account.Username, account.Password)

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionContains(t, account.UserID, otherSessionID1)
		requireUserSessionContains(t, account.UserID, otherSessionID2)

		cli := sessionClient(t, currentSessionID)

		rsp, err := cli.Delete[iam.SessionDeleteRsp](sessionsPath+"/others", nil)
		require.NoError(t, err)
		require.Equal(t, iam.SessionDeleteRsp{}, *rsp)

		list, err := cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		require.NoError(t, err)
		require.Len(t, list.Items, 1)
		require.Equal(t, 1, list.Total)
		require.Equal(t, currentSessionID, list.Items[0].ID)
		require.True(t, list.Items[0].IsCurrent)

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionNotContains(t, account.UserID, otherSessionID1)
		requireUserSessionNotContains(t, account.UserID, otherSessionID2)
		requireSessionNotFound(t, otherSessionID1)
		requireSessionNotFound(t, otherSessionID2)
	})

	t.Run("idempotent_when_no_other_sessions", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)

		cli := sessionClient(t, currentSessionID)

		rsp, err := cli.Delete[iam.SessionDeleteRsp](sessionsPath+"/others", nil)
		require.NoError(t, err)
		require.Equal(t, iam.SessionDeleteRsp{}, *rsp)

		list, err := cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		require.NoError(t, err)
		require.Len(t, list.Items, 1)
		require.Equal(t, 1, list.Total)
		require.Equal(t, currentSessionID, list.Items[0].ID)
		require.True(t, list.Items[0].IsCurrent)
	})
}

func TestSessionDeleteAll(t *testing.T) {
	clearSessionsAfterTest(t)

	t.Run("delete_all_sessions", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		otherSessionID := loginSession(t, account.Username, account.Password)

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionContains(t, account.UserID, otherSessionID)

		cli := sessionClient(t, currentSessionID)

		rsp, err := cli.Delete[iam.SessionDeleteAllRsp](sessionsPath, nil)
		require.NoError(t, err)
		require.Equal(t, iam.SessionDeleteAllRsp{}, *rsp)

		requireSessionNotFound(t, currentSessionID)
		requireSessionNotFound(t, otherSessionID)
		requireUserSessionNotContains(t, account.UserID, currentSessionID)
		requireUserSessionNotContains(t, account.UserID, otherSessionID)

		_, err = cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})

	t.Run("idempotent_when_stale_session_index_exists", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		staleSessionID := loginSession(t, account.Username, account.Password)

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionContains(t, account.UserID, staleSessionID)

		require.NoError(t, redis.Cache[modeliamsession.Session]().Delete(t.Context(), modeliamsession.SessionIDKey(staleSessionID)))
		requireUserSessionContains(t, account.UserID, staleSessionID)

		cli := sessionClient(t, currentSessionID)

		rsp, err := cli.Delete[iam.SessionDeleteAllRsp](sessionsPath, nil)
		require.NoError(t, err)
		require.Equal(t, iam.SessionDeleteAllRsp{}, *rsp)

		requireSessionNotFound(t, currentSessionID)
		requireSessionNotFound(t, staleSessionID)
		requireUserSessionNotContains(t, account.UserID, currentSessionID)
		requireUserSessionNotContains(t, account.UserID, staleSessionID)

		_, err = cli.Get[client.ListResult[iam.SessionView]](sessionsPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})
}

// requestMetadataProbeRsp reports the request metadata found on the context the
// session middleware handed to the handler.
type requestMetadataProbeRsp struct {
	Route    string `json:"route"`
	Path     string `json:"path"`
	RawQuery string `json:"raw_query"`
}

// registerRequestMetadataProbe registers the route TestSessionMiddlewareCarriesRequestMetadata
// reads the middleware's context through. It is registered on the authenticated
// group, which is where the session middleware runs, and it is the suite's own
// route rather than a module route so that the assertions describe the
// middleware alone instead of whatever a real endpoint does with the context.
func registerRequestMetadataProbe() error {
	router.Auth().GET(requestMetadataProbeGroupRoute, func(c *gin.Context) {
		meta := requestctx.FromContext(c.Request.Context())
		internalresponse.JSON(c, internalresponse.CodeSuccess, requestMetadataProbeRsp{
			Route:    meta.Route(),
			Path:     meta.Path(),
			RawQuery: meta.RawQuery(),
		})
	})

	return nil
}

func loadStoredSession(t *testing.T, sessionID string) modeliamsession.Session {
	t.Helper()

	session, err := redis.Cache[modeliamsession.Session]().Get(t.Context(), modeliamsession.SessionIDKey(sessionID))
	require.NoError(t, err)
	return session
}

func setSessionLastSeenAt(t *testing.T, sessionID string, lastSeenAt time.Time) modeliamsession.Session {
	t.Helper()

	session := loadStoredSession(t, sessionID)
	session.LastSeenAt = lastSeenAt.UTC()
	ttl := time.Until(session.ExpiresAt)
	require.Greater(t, ttl, time.Duration(0))
	require.NoError(t, redis.Cache[modeliamsession.Session]().Set(t.Context(), modeliamsession.SessionIDKey(sessionID), session, ttl))
	return session
}

func setSessionTenantID(t *testing.T, sessionID string, tenantID string) modeliamsession.Session {
	t.Helper()

	session := loadStoredSession(t, sessionID)
	session.TenantID = tenantID
	ttl := time.Until(session.ExpiresAt)
	require.Greater(t, ttl, time.Duration(0))
	require.NoError(t, redis.Cache[modeliamsession.Session]().Set(t.Context(), modeliamsession.SessionIDKey(sessionID), session, ttl))
	return session
}

func requireSessionNotFound(t *testing.T, sessionID string) {
	t.Helper()

	sessionKey := modeliamsession.SessionIDKey(sessionID)
	_, err := redis.Cache[modeliamsession.Session]().Get(t.Context(), sessionKey)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
}

func requireUserSessionContains(t *testing.T, userID, sessionID string) {
	t.Helper()

	userSessionIDs, err := redis.ZRange(t.Context(), modeliamsession.SessionUserKey(userID), 0, -1)
	require.NoError(t, err)
	require.Contains(t, userSessionIDs, sessionID)
}

func requireUserSessionNotContains(t *testing.T, userID, sessionID string) {
	t.Helper()

	userSessionIDs, err := redis.ZRange(t.Context(), modeliamsession.SessionUserKey(userID), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, userSessionIDs, sessionID)
}

func requireAllSessionContains(t *testing.T, sessionID string) {
	t.Helper()

	sessionIDs, err := redis.ZRange(t.Context(), modeliamsession.SessionAllKey(), 0, -1)
	require.NoError(t, err)
	require.Contains(t, sessionIDs, sessionID)
}

func requireAllSessionNotContains(t *testing.T, sessionID string) {
	t.Helper()

	sessionIDs, err := redis.ZRange(t.Context(), modeliamsession.SessionAllKey(), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, sessionIDs, sessionID)
}

func requireUserStateCacheCleared(t *testing.T, userID string) {
	t.Helper()

	_, err := redis.Get(t.Context(), modeliamsession.SessionUserStateKey(userID))
	require.ErrorIs(t, err, redis.ErrKeyNotExists)
}

func requireLastSeenSessionNotContains(t *testing.T, sessionID string) {
	t.Helper()

	sessionIDs, err := redis.ZRange(t.Context(), modeliamsession.SessionLastSeenKey(), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, sessionIDs, sessionID)
}

func sessionSetUserStatus(t *testing.T, username string, status modeliamuser.UserStatus) {
	t.Helper()

	users := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](context.Background()).WithQuery(&iam.User{Username: username}).List(&users))
	require.Len(t, users, 1)

	users[0].Status = status
	require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect(colUsername, colUserStatus).Update(users[0]))
}

func rootSessionTestAccount() sessionTestAccount {
	return sessionTestAccount{
		UserID:   consts.AUTHZ_USER_ROOT,
		Username: consts.AUTHZ_USER_ROOT,
		Password: rootPassword,
	}
}

func newSessionTestAccount(t *testing.T) sessionTestAccount {
	t.Helper()

	username := fmt.Sprintf("session_%d", time.Now().UnixNano())
	password := "12345678"

	cli, err := client.New(baseURL)
	require.NoError(t, err)

	rsp, err := cli.Post[iam.SignupRsp](signupPath, iam.SignupReq{
		Username:   username,
		Password:   password,
		RePassword: password,
	})
	require.NoError(t, err)

	account := sessionTestAccount{
		Username: username,
		Password: password,
	}
	require.Equal(t, username, rsp.Username)
	require.NotEmpty(t, rsp.UserID)
	require.NotEmpty(t, rsp.Message)
	account.UserID = rsp.UserID

	return account
}

func loginSession(t *testing.T, username, password string) string {
	t.Helper()

	return loginSessionIDFromCookie(t, username, password)
}

func sessionLoginRoot(t *testing.T) string {
	t.Helper()

	sessionID := loginSession(t, consts.AUTHZ_USER_ROOT, rootPassword)
	t.Cleanup(func() {
		modeliamsession.InvalidateUserSessions(context.Background(), consts.AUTHZ_USER_ROOT)
	})
	return sessionID
}

func loginSessionIDFromCookie(t *testing.T, username, password string) string {
	t.Helper()

	cli, err := client.New(baseURL)
	require.NoError(t, err)

	apiResp, err := cli.Do(http.MethodPost, loginPath, iam.LoginReq{
		Username: username,
		Password: password,
	})
	require.NoError(t, err)

	rsp := testutil.DecodeResp[iam.LoginRsp](t, apiResp)
	require.False(t, rsp.ServerTime.IsZero())
	require.Equal(t, modeliamsession.SessionStatusActive, rsp.Session.Status)
	require.False(t, rsp.Session.IssuedAt.IsZero())
	require.False(t, rsp.Session.LastSeenAt.IsZero())
	require.False(t, rsp.Session.ExpiresAt.IsZero())
	require.Positive(t, rsp.Session.ExpiresInSeconds)
	require.True(t, rsp.Session.ExpiresAt.After(rsp.ServerTime))
	require.NotEmpty(t, rsp.Principal.UserID)
	require.Equal(t, username, rsp.Principal.Username)

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(apiResp.Data, &data), "response data: %s", string(apiResp.Data))
	require.NotContains(t, data, "session_id")

	cookie := apiResp.Cookie("session_id")
	require.NotNil(t, cookie, "session cookie not found")
	require.NotEmpty(t, cookie.Value)
	require.Regexp(t, `^[0-9a-f]{64}$`, cookie.Value)
	return cookie.Value
}

// clearSessionsAfterTest drops every session key once the test is done. Session
// state is global to the package: the admin list endpoints read the process
// wide session and last-seen indexes, so one test's leftovers are another
// test's phantom rows. The redis container is fresh per run, so this is about
// isolating the tests from each other, not about earlier runs.
func clearSessionsAfterTest(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		require.NoError(t, redis.RemovePrefix(context.Background(), modeliamsession.SessionNamespacePrefix))
	})
}
