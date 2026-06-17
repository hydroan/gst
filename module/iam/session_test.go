package iam_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/helper"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

var (
	sessionsAPI      = fmt.Sprintf("http://localhost:%d/api/iam/sessions", port)
	adminSessionsAPI = fmt.Sprintf("http://localhost:%d/api/iam/admin/sessions", port)
	heartbeatAPI     = fmt.Sprintf("http://localhost:%d/api/iam/session/heartbeat", port)
	onlineuserAPI    = fmt.Sprintf("http://localhost:%d/api/online-users", port)
)

type sessionTestAccount struct {
	UserID   string
	Username string
	Password string
}

func TestSessionHeartbeat(t *testing.T) {
	setupSessionRedisCleanup(t)

	account := newSessionTestAccount(t)
	sessionID := loginSession(t, account.Username, account.Password)

	sessionKey := modeliamsession.SessionIDKey(sessionID)
	before, err := redis.Cache[modeliamsession.Session]().Get(sessionKey)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	cli, err := client.New(heartbeatAPI, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)

	resp, err := cli.Create(nil)
	require.NoError(t, err)

	helper.TestResp[*iam.Heartbeat](t, resp, func(t *testing.T, rsp *iam.Heartbeat) { t.Helper() })

	after, err := redis.Cache[modeliamsession.Session]().Get(sessionKey)
	require.NoError(t, err)
	require.Equal(t, before.ExpiresAt, after.ExpiresAt)
	require.Equal(t, before.LastSeenAt, after.LastSeenAt)
}

func TestSessionCurrent(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("get_current_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(currentAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, new(struct{}))
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp iam.CurrentListRsp) {
			t.Helper()
			require.NotEmpty(t, rsp.Principal.UserID)
			require.Equal(t, account.Username, rsp.Principal.Username)
			require.Equal(t, string(modeliamuser.UserStatusActive), rsp.Principal.Status)
			require.False(t, rsp.Principal.MustChangePassword)
			require.True(t, rsp.Session.IsCurrent)
			require.Equal(t, sessionID, rsp.Session.ID)
		})
	})
}

func TestSessionGet(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("get_current_user_session_detail", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		otherSessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		got := new(iam.SessionsGetRsp)
		resp, err := cli.Get(otherSessionID, got)
		require.NoError(t, err)
		helper.TestResp[*iam.SessionsGetRsp](t, resp, func(t *testing.T, rsp *iam.SessionsGetRsp) {
			t.Helper()
			require.Equal(t, otherSessionID, rsp.Session.ID)
			require.False(t, rsp.Session.IsCurrent)
		})
	})

	t.Run("get_current_session_detail", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		got := new(iam.SessionsGetRsp)
		resp, err := cli.Get(currentSessionID, got)
		require.NoError(t, err)
		helper.TestResp[*iam.SessionsGetRsp](t, resp, func(t *testing.T, rsp *iam.SessionsGetRsp) {
			t.Helper()
			require.Equal(t, currentSessionID, rsp.Session.ID)
			require.True(t, rsp.Session.IsCurrent)
		})
	})

	t.Run("forbidden_when_getting_other_user_session", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		victimSessionID := loginSession(t, victim.Username, victim.Password)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: attackerSessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Get(victimSessionID, new(iam.SessionsGetRsp))
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
	})

	t.Run("not_found_when_session_missing", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Get("missing-session-id", new(iam.SessionsGetRsp))
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
	})
}

func TestSessionList(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("list_current_user_sessions", func(t *testing.T) {
		account := newSessionTestAccount(t)
		otherSessionID := loginSession(t, account.Username, account.Password)
		currentSessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		items := make([]iam.SessionView, 0)
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[iam.SessionView]) {
			t.Helper()
			require.Len(t, rsp.Items, 2)
			require.EqualValues(t, 2, rsp.Total)

			sessionMap := make(map[string]iam.SessionView, len(rsp.Items))
			for i := range rsp.Items {
				sessionMap[rsp.Items[i].ID] = rsp.Items[i]
			}

			require.Contains(t, sessionMap, currentSessionID)
			require.Contains(t, sessionMap, otherSessionID)
			require.True(t, sessionMap[currentSessionID].IsCurrent)
			require.False(t, sessionMap[otherSessionID].IsCurrent)
		})
	})
}

func TestAdminSessionList(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("list_all_sessions_grouped_by_user", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		firstUser := newSessionTestAccount(t)
		firstUserSessionID1 := loginSession(t, firstUser.Username, firstUser.Password)
		firstUserSessionID2 := loginSession(t, firstUser.Username, firstUser.Password)

		secondUser := newSessionTestAccount(t)
		secondUserSessionID := loginSession(t, secondUser.Username, secondUser.Password)

		requireAllSessionContains(t, adminSessionID)
		requireAllSessionContains(t, firstUserSessionID1)
		requireAllSessionContains(t, firstUserSessionID2)
		requireAllSessionContains(t, secondUserSessionID)

		cli, err := client.New(adminSessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)

		items := make([]iam.AdminSessionUserView, 0)
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp iam.AdminSessionsListRsp) {
			t.Helper()
			require.GreaterOrEqual(t, rsp.Total, int64(3))
			require.GreaterOrEqual(t, rsp.SessionTotal, int64(4))

			userMap := make(map[string]iam.AdminSessionUserView, len(rsp.Items))
			for i := range rsp.Items {
				userMap[rsp.Items[i].Username] = rsp.Items[i]
			}

			require.Contains(t, userMap, adminAccount.Username)
			require.Contains(t, userMap, firstUser.Username)
			require.Contains(t, userMap, secondUser.Username)

			require.EqualValues(t, 1, userMap[adminAccount.Username].SessionTotal)
			require.Len(t, userMap[adminAccount.Username].Sessions, 1)
			require.Equal(t, adminSessionID, userMap[adminAccount.Username].Sessions[0].ID)
			require.False(t, userMap[adminAccount.Username].Sessions[0].IsCurrent)

			firstUserSessionIDs := make(map[string]struct{}, len(userMap[firstUser.Username].Sessions))
			for i := range userMap[firstUser.Username].Sessions {
				firstUserSessionIDs[userMap[firstUser.Username].Sessions[i].ID] = struct{}{}
				require.False(t, userMap[firstUser.Username].Sessions[i].IsCurrent)
			}
			require.EqualValues(t, 2, userMap[firstUser.Username].SessionTotal)
			require.Len(t, userMap[firstUser.Username].Sessions, 2)
			_, ok := firstUserSessionIDs[firstUserSessionID1]
			require.True(t, ok)
			_, ok = firstUserSessionIDs[firstUserSessionID2]
			require.True(t, ok)

			require.EqualValues(t, 1, userMap[secondUser.Username].SessionTotal)
			require.Len(t, userMap[secondUser.Username].Sessions, 1)
			require.Equal(t, secondUserSessionID, userMap[secondUser.Username].Sessions[0].ID)
			require.False(t, userMap[secondUser.Username].Sessions[0].IsCurrent)
		})
	})

	t.Run("forbidden_for_regular_user", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(adminSessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		_, err = cli.List(new([]iam.AdminSessionUserView), new(int64))
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
	})
}

func TestAdminSessionGet(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("get_other_user_session_detail", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		targetAccount := newSessionTestAccount(t)
		targetSessionID := loginSession(t, targetAccount.Username, targetAccount.Password)

		cli, err := client.New(adminSessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Get(targetSessionID, new(modeliamsession.AdminSessionsGetRsp))
		require.NoError(t, err)
		helper.TestResp[*modeliamsession.AdminSessionsGetRsp](t, resp, func(t *testing.T, rsp *modeliamsession.AdminSessionsGetRsp) {
			t.Helper()
			require.Equal(t, targetSessionID, rsp.Session.ID)
			require.False(t, rsp.Session.IsCurrent)
			require.NotEmpty(t, rsp.Session.ClientIP)
			require.NotEmpty(t, rsp.Session.UserAgent)
		})
	})

	t.Run("forbidden_for_regular_user", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		victimSessionID := loginSession(t, victim.Username, victim.Password)

		cli, err := client.New(adminSessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: attackerSessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Get(victimSessionID, new(modeliamsession.AdminSessionsGetRsp))
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
	})

	t.Run("not_found_when_session_missing", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		cli, err := client.New(adminSessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Get("missing-session-id", new(modeliamsession.AdminSessionsGetRsp))
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
	})
}

func TestAdminSessionDelete(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("delete_other_user_session", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		targetAccount := newSessionTestAccount(t)
		targetSessionID := loginSession(t, targetAccount.Username, targetAccount.Password)

		requireUserSessionContains(t, targetAccount.UserID, targetSessionID)
		requireAllSessionContains(t, targetSessionID)

		cli, err := client.New(adminSessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Delete(targetSessionID)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.AdminSessionsDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.AdminSessionsDeleteRsp{}, rsp)
		})

		requireSessionNotFound(t, targetSessionID)
		requireUserSessionNotContains(t, targetAccount.UserID, targetSessionID)
	})

	t.Run("forbidden_for_regular_user", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		victimSessionID := loginSession(t, victim.Username, victim.Password)
		requireUserSessionContains(t, victim.UserID, victimSessionID)

		cli, err := client.New(adminSessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: attackerSessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Delete(victimSessionID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")

		requireUserSessionContains(t, victim.UserID, victimSessionID)
	})

	t.Run("not_found_when_session_missing", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		cli, err := client.New(adminSessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: adminSessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Delete("missing-session-id")
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
	})
}

func TestAdminUserSessionsList(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("list_all_sessions_of_target_user", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		targetAccount := newSessionTestAccount(t)
		targetSessionID1 := loginSession(t, targetAccount.Username, targetAccount.Password)
		targetSessionID2 := loginSession(t, targetAccount.Username, targetAccount.Password)

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, targetAccount.UserID),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}),
		)
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, new(struct{}))
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.AdminUserSessionsListRsp) {
			t.Helper()
			require.Equal(t, targetAccount.UserID, rsp.User.UserID)
			require.Equal(t, targetAccount.Username, rsp.User.Username)
			require.EqualValues(t, 2, rsp.User.SessionTotal)
			require.Len(t, rsp.User.Sessions, 2)

			sessionMap := make(map[string]iam.SessionView, len(rsp.User.Sessions))
			for i := range rsp.User.Sessions {
				sessionMap[rsp.User.Sessions[i].ID] = rsp.User.Sessions[i]
				require.False(t, rsp.User.Sessions[i].IsCurrent)
			}

			require.Contains(t, sessionMap, targetSessionID1)
			require.Contains(t, sessionMap, targetSessionID2)
		})
	})

	t.Run("forbidden_for_regular_user", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		_ = loginSession(t, victim.Username, victim.Password)

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, victim.UserID),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: attackerSessionID,
			}),
		)
		require.NoError(t, err)

		_, err = cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
	})

	t.Run("not_found_when_user_missing", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, "missing-user-id"),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}),
		)
		require.NoError(t, err)

		_, err = cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
	})

	t.Run("list_target_user_with_no_sessions", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		targetAccount := newSessionTestAccount(t)

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, targetAccount.UserID),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}),
		)
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, new(struct{}))
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.AdminUserSessionsListRsp) {
			t.Helper()
			require.Equal(t, targetAccount.UserID, rsp.User.UserID)
			require.Equal(t, targetAccount.Username, rsp.User.Username)
			require.Zero(t, rsp.User.SessionTotal)
			require.Empty(t, rsp.User.Sessions)
		})
	})

	t.Run("mark_current_session_when_admin_views_self", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		currentAdminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)
		otherAdminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, adminAccount.UserID),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: currentAdminSessionID,
			}),
		)
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodGet, new(struct{}))
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.AdminUserSessionsListRsp) {
			t.Helper()
			require.EqualValues(t, 2, rsp.User.SessionTotal)
			require.Len(t, rsp.User.Sessions, 2)

			sessionMap := make(map[string]iam.SessionView, len(rsp.User.Sessions))
			for i := range rsp.User.Sessions {
				sessionMap[rsp.User.Sessions[i].ID] = rsp.User.Sessions[i]
			}

			require.True(t, sessionMap[currentAdminSessionID].IsCurrent)
			require.False(t, sessionMap[otherAdminSessionID].IsCurrent)
		})
	})
}

func TestAdminUserSessionsDelete(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("delete_all_sessions_of_target_user", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		targetAccount := newSessionTestAccount(t)
		targetSessionID1 := loginSession(t, targetAccount.Username, targetAccount.Password)
		targetSessionID2 := loginSession(t, targetAccount.Username, targetAccount.Password)

		requireUserSessionContains(t, targetAccount.UserID, targetSessionID1)
		requireUserSessionContains(t, targetAccount.UserID, targetSessionID2)
		requireAllSessionContains(t, targetSessionID1)
		requireAllSessionContains(t, targetSessionID2)

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, targetAccount.UserID),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}),
		)
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodDelete, new(struct{}))
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.AdminUserSessionsDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.AdminUserSessionsDeleteRsp{}, rsp)
		})

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

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, victim.UserID),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: attackerSessionID,
			}),
		)
		require.NoError(t, err)

		_, err = cli.Request(http.MethodDelete, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")

		requireUserSessionContains(t, victim.UserID, victimSessionID)
	})

	t.Run("not_found_when_user_missing", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, "missing-user-id"),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}),
		)
		require.NoError(t, err)

		_, err = cli.Request(http.MethodDelete, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
	})

	t.Run("idempotent_when_target_user_has_no_sessions", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		adminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		targetAccount := newSessionTestAccount(t)

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, targetAccount.UserID),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: adminSessionID,
			}),
		)
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodDelete, new(struct{}))
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.AdminUserSessionsDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.AdminUserSessionsDeleteRsp{}, rsp)
		})
	})

	t.Run("delete_all_sessions_of_current_admin", func(t *testing.T) {
		adminAccount := newSessionTestAccount(t)
		sessionSetSuperuser(t, adminAccount.Username, true)
		currentAdminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)
		otherAdminSessionID := loginSession(t, adminAccount.Username, adminAccount.Password)

		requireUserSessionContains(t, adminAccount.UserID, currentAdminSessionID)
		requireUserSessionContains(t, adminAccount.UserID, otherAdminSessionID)

		cli, err := client.New(
			fmt.Sprintf("http://localhost:%d/api/iam/admin/users/%s/sessions", port, adminAccount.UserID),
			client.WithCookie(&http.Cookie{
				Name:  "session_id",
				Value: currentAdminSessionID,
			}),
		)
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodDelete, new(struct{}))
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.AdminUserSessionsDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.AdminUserSessionsDeleteRsp{}, rsp)
		})

		requireSessionNotFound(t, currentAdminSessionID)
		requireSessionNotFound(t, otherAdminSessionID)
		requireUserSessionNotContains(t, adminAccount.UserID, currentAdminSessionID)
		requireUserSessionNotContains(t, adminAccount.UserID, otherAdminSessionID)
		requireAllSessionNotContains(t, currentAdminSessionID)
		requireAllSessionNotContains(t, otherAdminSessionID)

		currentCli, err := client.New(currentAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentAdminSessionID,
		}))
		require.NoError(t, err)

		_, err = currentCli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "401")
	})
}

func TestSessionOnlineUsers(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("list_online_users", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(onlineuserAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		items := make([]*iam.OnlineUser, 0)
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*iam.OnlineUser]) {
			t.Helper()
			require.NotEmpty(t, rsp.Items)
		})
	})
}

func TestSessionDelete(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("delete_non_current_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		otherSessionID := loginSession(t, account.Username, account.Password)

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionContains(t, account.UserID, otherSessionID)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Delete(otherSessionID)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.SessionsDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.SessionsDeleteRsp{}, rsp)
		})

		items := make([]iam.SessionView, 0)
		total := new(int64)
		resp, err = cli.List(&items, total)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[iam.SessionView]) {
			t.Helper()
			require.Len(t, rsp.Items, 1)
			require.EqualValues(t, 1, rsp.Total)
			require.Equal(t, currentSessionID, rsp.Items[0].ID)
			require.True(t, rsp.Items[0].IsCurrent)
		})

		requireSessionNotFound(t, otherSessionID)
		requireUserSessionNotContains(t, account.UserID, otherSessionID)
		requireUserSessionContains(t, account.UserID, currentSessionID)
	})

	t.Run("delete_missing_session_is_idempotent", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		missingSessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Delete(missingSessionID)
		require.NoError(t, err)

		resp, err := cli.Delete(missingSessionID)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.SessionsDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.SessionsDeleteRsp{}, rsp)
		})

		items := make([]iam.SessionView, 0)
		total := new(int64)
		resp, err = cli.List(&items, total)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[iam.SessionView]) {
			t.Helper()
			require.Len(t, rsp.Items, 1)
			require.EqualValues(t, 1, rsp.Total)
			require.Equal(t, currentSessionID, rsp.Items[0].ID)
			require.True(t, rsp.Items[0].IsCurrent)
		})
	})

	t.Run("forbidden_when_deleting_other_user_session", func(t *testing.T) {
		attacker := newSessionTestAccount(t)
		attackerSessionID := loginSession(t, attacker.Username, attacker.Password)

		victim := newSessionTestAccount(t)
		victimSessionID := loginSession(t, victim.Username, victim.Password)
		requireUserSessionContains(t, victim.UserID, victimSessionID)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: attackerSessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Delete(victimSessionID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")

		requireUserSessionContains(t, victim.UserID, victimSessionID)
	})

	t.Run("delete_current_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		requireUserSessionContains(t, account.UserID, sessionID)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Delete(sessionID)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.SessionsDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.SessionsDeleteRsp{}, rsp)
		})

		requireSessionNotFound(t, sessionID)
		requireUserSessionNotContains(t, account.UserID, sessionID)

		currentCli, err := client.New(currentAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		_, err = currentCli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "401")
	})
}

func TestSessionDeleteOthers(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("delete_all_other_sessions", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		otherSessionID1 := loginSession(t, account.Username, account.Password)
		otherSessionID2 := loginSession(t, account.Username, account.Password)

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionContains(t, account.UserID, otherSessionID1)
		requireUserSessionContains(t, account.UserID, otherSessionID2)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Delete("others")
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.SessionsDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.SessionsDeleteRsp{}, rsp)
		})

		items := make([]iam.SessionView, 0)
		total := new(int64)
		resp, err = cli.List(&items, total)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[iam.SessionView]) {
			t.Helper()
			require.Len(t, rsp.Items, 1)
			require.EqualValues(t, 1, rsp.Total)
			require.Equal(t, currentSessionID, rsp.Items[0].ID)
			require.True(t, rsp.Items[0].IsCurrent)
		})

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionNotContains(t, account.UserID, otherSessionID1)
		requireUserSessionNotContains(t, account.UserID, otherSessionID2)
		requireSessionNotFound(t, otherSessionID1)
		requireSessionNotFound(t, otherSessionID2)
	})

	t.Run("idempotent_when_no_other_sessions", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Delete("others")
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.SessionsDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.SessionsDeleteRsp{}, rsp)
		})

		items := make([]iam.SessionView, 0)
		total := new(int64)
		resp, err = cli.List(&items, total)
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp ListResponse[iam.SessionView]) {
			t.Helper()
			require.Len(t, rsp.Items, 1)
			require.EqualValues(t, 1, rsp.Total)
			require.Equal(t, currentSessionID, rsp.Items[0].ID)
			require.True(t, rsp.Items[0].IsCurrent)
		})
	})
}

func TestSessionDeleteAll(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("delete_all_sessions", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		otherSessionID := loginSession(t, account.Username, account.Password)

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionContains(t, account.UserID, otherSessionID)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodDelete, new(struct{}))
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.SessionsDeleteAllRsp) {
			t.Helper()
			require.Equal(t, iam.SessionsDeleteAllRsp{}, rsp)
		})

		requireSessionNotFound(t, currentSessionID)
		requireSessionNotFound(t, otherSessionID)
		requireUserSessionNotContains(t, account.UserID, currentSessionID)
		requireUserSessionNotContains(t, account.UserID, otherSessionID)

		_, err = cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "401")
	})

	t.Run("idempotent_when_stale_session_index_exists", func(t *testing.T) {
		account := newSessionTestAccount(t)
		currentSessionID := loginSession(t, account.Username, account.Password)
		staleSessionID := loginSession(t, account.Username, account.Password)

		requireUserSessionContains(t, account.UserID, currentSessionID)
		requireUserSessionContains(t, account.UserID, staleSessionID)

		require.NoError(t, redis.Cache[modeliamsession.Session]().Delete(modeliamsession.SessionIDKey(staleSessionID)))
		requireUserSessionContains(t, account.UserID, staleSessionID)

		cli, err := client.New(sessionsAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: currentSessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodDelete, new(struct{}))
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.SessionsDeleteAllRsp) {
			t.Helper()
			require.Equal(t, iam.SessionsDeleteAllRsp{}, rsp)
		})

		requireSessionNotFound(t, currentSessionID)
		requireSessionNotFound(t, staleSessionID)
		requireUserSessionNotContains(t, account.UserID, currentSessionID)
		requireUserSessionNotContains(t, account.UserID, staleSessionID)

		_, err = cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "401")
	})
}

func TestSessionCurrentDelete(t *testing.T) {
	setupSessionRedisCleanup(t)

	t.Run("delete_current_session", func(t *testing.T) {
		account := newSessionTestAccount(t)
		sessionID := loginSession(t, account.Username, account.Password)
		requireUserSessionContains(t, account.UserID, sessionID)

		cli, err := client.New(currentAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Request(http.MethodDelete, new(struct{}))
		require.NoError(t, err)
		helper.TestResp(t, resp, func(t *testing.T, rsp iam.CurrentDeleteRsp) {
			t.Helper()
			require.Equal(t, iam.CurrentDeleteRsp{}, rsp)
		})

		requireSessionNotFound(t, sessionID)
		requireUserSessionNotContains(t, account.UserID, sessionID)

		_, err = cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "401")
	})
}

func setupSessionRedisCleanup(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		require.NoError(t, redis.RemovePrefix(modeliamsession.SessionNamespacePrefix))
	})
}

func requireSessionNotFound(t *testing.T, sessionID string) {
	t.Helper()

	sessionKey := modeliamsession.SessionIDKey(sessionID)
	_, err := redis.Cache[modeliamsession.Session]().Get(sessionKey)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
}

func requireUserSessionContains(t *testing.T, userID, sessionID string) {
	t.Helper()

	userSessionIDs, err := redis.ZRange(modeliamsession.SessionUserKey(userID), 0, -1)
	require.NoError(t, err)
	require.Contains(t, userSessionIDs, sessionID)
}

func requireUserSessionNotContains(t *testing.T, userID, sessionID string) {
	t.Helper()

	userSessionIDs, err := redis.ZRange(modeliamsession.SessionUserKey(userID), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, userSessionIDs, sessionID)
}

func requireAllSessionContains(t *testing.T, sessionID string) {
	t.Helper()

	sessionIDs, err := redis.ZRange(modeliamsession.SessionAllKey(), 0, -1)
	require.NoError(t, err)
	require.Contains(t, sessionIDs, sessionID)
}

func requireAllSessionNotContains(t *testing.T, sessionID string) {
	t.Helper()

	sessionIDs, err := redis.ZRange(modeliamsession.SessionAllKey(), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, sessionIDs, sessionID)
}

func sessionSetSuperuser(t *testing.T, username string, enabled bool) {
	t.Helper()

	users := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](nil).WithLimit(1).WithQuery(&iam.User{Username: username}).List(&users))
	require.Len(t, users, 1)

	users[0].IsSuperuser = &enabled
	require.NoError(t, database.Database[*iam.User](nil).Update(users[0]))
}

func newSessionTestAccount(t *testing.T) sessionTestAccount {
	t.Helper()

	username := fmt.Sprintf("session_%d", time.Now().UnixNano())
	password := "12345678"

	cli, err := client.New(signupAPI)
	require.NoError(t, err)

	resp, err := cli.Create(iam.SignupReq{
		Username:   username,
		Password:   password,
		RePassword: password,
	})
	require.NoError(t, err)

	account := sessionTestAccount{
		Username: username,
		Password: password,
	}
	helper.TestResp(t, resp, func(t *testing.T, rsp iam.SignupRsp) {
		t.Helper()
		require.Equal(t, username, rsp.Username)
		require.NotEmpty(t, rsp.UserID)
		require.NotEmpty(t, rsp.Message)
		account.UserID = rsp.UserID
	})

	return account
}

func loginSession(t *testing.T, username, password string) string {
	t.Helper()

	cli, err := client.New(loginAPI)
	require.NoError(t, err)

	resp, err := cli.Create(iam.LoginReq{
		Username: username,
		Password: password,
	})
	require.NoError(t, err)

	sessionID := ""
	helper.TestResp(t, resp, func(t *testing.T, rsp *iam.LoginRsp) {
		t.Helper()
		require.NotEmpty(t, rsp.SessionID)
		sessionID = rsp.SessionID
	})

	return sessionID
}
