package iam_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/iam"
	"github.com/hydroan/gst/provider/redis"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type accountTestUser struct {
	UserID    string
	Username  string
	Password  string
	SessionID string
}

func accountSignupUser(t *testing.T, prefix, password string) accountTestUser {
	t.Helper()

	user := accountTestUser{
		Username: fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()),
		Password: password,
	}

	cli, err := client.New(signupAPI)
	require.NoError(t, err)

	resp, err := cli.Create(iam.SignupReq{
		Username:   user.Username,
		Password:   user.Password,
		RePassword: user.Password,
	})
	require.NoError(t, err)

	testutil.TestResp(t, resp, func(t *testing.T, rsp iam.SignupRsp) {
		t.Helper()
		require.Equal(t, user.Username, rsp.Username)
		require.NotEmpty(t, rsp.UserID)
		require.NotEmpty(t, rsp.Message)
		user.UserID = rsp.UserID
	})

	t.Cleanup(func() {
		accountCleanupUser(t, user.Username)
	})

	return user
}

func accountCleanupUser(t *testing.T, username string) {
	t.Helper()

	users := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](context.Background()).WithQuery(&iam.User{Username: username}).List(&users))
	if len(users) == 0 {
		return
	}

	for _, user := range users {
		serviceiamsession.InvalidateUserSessions(t.Context(), user.ID)
	}
	require.NoError(t, database.Database[*iam.User](context.Background()).Delete(users...))
}

func accountLoginUser(t *testing.T, user *accountTestUser, password string) string {
	t.Helper()

	return loginSessionIDFromCookie(t, user.Username, password)
}

func accountRequireSessionNotFound(t *testing.T, sessionID string) {
	t.Helper()

	sessionKey := modeliamsession.SessionIDKey(sessionID)
	_, err := redis.Cache[modeliamsession.Session]().WithContext(t.Context()).Get(sessionKey)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
}

func accountRequireUserSessionContains(t *testing.T, userID, sessionID string) {
	t.Helper()

	userSessionIDs, err := redis.ZRange(t.Context(), modeliamsession.SessionUserKey(userID), 0, -1)
	require.NoError(t, err)
	require.Contains(t, userSessionIDs, sessionID)
}

func accountRequireUserSessionNotContains(t *testing.T, userID, sessionID string) {
	t.Helper()

	userSessionIDs, err := redis.ZRange(t.Context(), modeliamsession.SessionUserKey(userID), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, userSessionIDs, sessionID)
}

func accountSetSuperuser(t *testing.T, username string, enabled bool) {
	t.Helper()

	actors := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](context.Background()).WithLimit(1).WithQuery(&iam.User{Username: username}).List(&actors))
	require.Len(t, actors, 1)

	actors[0].IsSuperuser = &enabled
	require.NoError(t, database.Database[*iam.User](context.Background()).Update(actors[0]))
}

func TestAccountSignup(t *testing.T) {
	user := accountSignupUser(t, "acct_signup", "12345678")

	require.NotEmpty(t, user.UserID)
	require.NotEmpty(t, user.Username)
}

func TestAccountLogin(t *testing.T) {
	t.Run("login", func(t *testing.T) {
		user := accountSignupUser(t, "acct_login", "12345678")
		user.SessionID = accountLoginUser(t, &user, user.Password)

		require.NotEmpty(t, user.SessionID)
		accountRequireUserSessionContains(t, user.UserID, user.SessionID)
	})

	t.Run("returns_authenticated_session", func(t *testing.T) {
		user := accountSignupUser(t, "acct_login_response", "12345678")

		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		resp, err := cli.Create(iam.LoginReq{
			Username: user.Username,
			Password: user.Password,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.LoginRsp) {
			t.Helper()

			require.False(t, rsp.ServerTime.IsZero())
			require.Equal(t, modeliamsession.SessionStatusActive, rsp.Session.Status)
			require.False(t, rsp.Session.IssuedAt.IsZero())
			require.False(t, rsp.Session.LastSeenAt.IsZero())
			require.False(t, rsp.Session.ExpiresAt.IsZero())
			require.Positive(t, rsp.Session.ExpiresInSeconds)
			require.True(t, rsp.Session.ExpiresAt.After(rsp.ServerTime))
			require.Equal(t, user.UserID, rsp.Principal.UserID)
			require.Equal(t, user.Username, rsp.Principal.Username)
			require.False(t, rsp.Principal.MustChangePassword)
		})
	})

	t.Run("sets_session_cookie", func(t *testing.T) {
		user := accountSignupUser(t, "acct_login_cookie", "12345678")
		cookie := loginSessionCookie(t, user.Username, user.Password)

		require.Equal(t, "session_id", cookie.Name)
		require.NotEmpty(t, cookie.Value)
		require.Equal(t, "/", cookie.Path)
		require.True(t, cookie.HttpOnly)
		require.True(t, cookie.Secure)
		require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
		require.Positive(t, cookie.MaxAge)
	})

	t.Run("updates_login_statistics_after_successful_session_create", func(t *testing.T) {
		user := accountSignupUser(t, "acct_login_stats", "12345678")

		users := make([]*iam.User, 0)
		require.NoError(t, database.Database[*iam.User](context.Background()).
			WithLimit(1).
			WithQuery(&iam.User{Username: user.Username}).
			List(&users))
		require.Len(t, users, 1)

		loginCount := 2
		users[0].LoginCount = &loginCount
		users[0].FailedLoginCount = 3
		require.NoError(t, database.Database[*iam.User](context.Background()).
			WithoutHook().
			WithSelect("username", "login_count", "failed_login_count").
			Update(users[0]))

		sessionID := accountLoginUser(t, &user, user.Password)
		accountRequireUserSessionContains(t, user.UserID, sessionID)

		got := new(iam.User)
		require.NoError(t, database.Database[*iam.User](context.Background()).Get(got, user.UserID))
		require.NotNil(t, got.LastLoginAt)
		require.NotNil(t, got.LastLoginIP)
		require.NotEmpty(t, *got.LastLoginIP)
		require.NotNil(t, got.LoginCount)
		require.Equal(t, loginCount+1, *got.LoginCount)
		require.Zero(t, got.FailedLoginCount)
	})
}

func TestAccountLogout(t *testing.T) {
	user := accountSignupUser(t, "acct_logout", "12345678")
	user.SessionID = accountLoginUser(t, &user, user.Password)
	accountRequireUserSessionContains(t, user.UserID, user.SessionID)

	t.Run("logout", func(t *testing.T) {
		cli, err := client.New(logoutAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: user.SessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(nil)
		require.NoError(t, err)

		testutil.TestResp[*iam.LogoutRsp](t, resp, func(t *testing.T, rsp *iam.LogoutRsp) {
			t.Helper()
			require.NotEmpty(t, rsp.Msg)
		})

		accountRequireSessionNotFound(t, user.SessionID)
		accountRequireUserSessionNotContains(t, user.UserID, user.SessionID)
	})

	t.Run("users_unauthorized_after_logout", func(t *testing.T) {
		cli, err := client.New(userAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: user.SessionID,
		}))
		require.NoError(t, err)

		items := make([]*iam.User, 0)
		total := new(int64)
		_, err = cli.List(&items, total)
		require.Error(t, err)
	})

	t.Run("login_again", func(t *testing.T) {
		user.SessionID = accountLoginUser(t, &user, user.Password)
		require.NotEmpty(t, user.SessionID)
	})

	t.Run("returns_error_when_session_index_delete_fails", func(t *testing.T) {
		brokenIndexUser := accountSignupUser(t, "acct_logout_broken_index", "12345678")
		brokenIndexUser.SessionID = accountLoginUser(t, &brokenIndexUser, brokenIndexUser.Password)

		userSessionKey := modeliamsession.SessionUserKey(brokenIndexUser.UserID)
		t.Cleanup(func() {
			require.NoError(t, redis.Del(context.Background(), userSessionKey, modeliamsession.SessionIDKey(brokenIndexUser.SessionID)))
			require.NoError(t, redis.ZRem(context.Background(), modeliamsession.SessionAllKey(), brokenIndexUser.SessionID))
			require.NoError(t, redis.ZRem(context.Background(), modeliamsession.SessionLastSeenKey(), brokenIndexUser.SessionID))
			serviceiamsession.InvalidateUserSessions(context.Background(), brokenIndexUser.UserID)
		})

		require.NoError(t, redis.Del(t.Context(), userSessionKey))
		require.NoError(t, redis.Set(t.Context(), userSessionKey, "not-a-zset", time.Hour))

		cli, err := client.New(logoutAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: brokenIndexUser.SessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Create(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "500")
		require.Contains(t, err.Error(), "failed to logout")
	})
}

func TestAccountChangePassword(t *testing.T) {
	user := accountSignupUser(t, "acct_changepwd", "12345678")
	newPassword := "123456789"
	user.SessionID = accountLoginUser(t, &user, user.Password)
	otherSessionID := accountLoginUser(t, &user, user.Password)
	accountRequireUserSessionContains(t, user.UserID, user.SessionID)
	accountRequireUserSessionContains(t, user.UserID, otherSessionID)

	t.Run("change_password", func(t *testing.T) {
		cli, err := client.New(changepasswordAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: user.SessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(iam.ChangePasswordReq{
			OldPassword: user.Password,
			NewPassword: newPassword,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp *iam.ChangePasswordRsp) {
			t.Helper()
			require.NotEmpty(t, rsp.Msg)
		})
	})

	t.Run("keeps_current_session_and_revokes_other_sessions", func(t *testing.T) {
		accountRequireUserSessionContains(t, user.UserID, user.SessionID)
		accountRequireSessionNotFound(t, otherSessionID)
		accountRequireUserSessionNotContains(t, user.UserID, otherSessionID)
	})

	t.Run("login_with_new_password", func(t *testing.T) {
		user.SessionID = accountLoginUser(t, &user, newPassword)
		require.NotEmpty(t, user.SessionID)
	})

	t.Run("user_module_forbidden_with_new_session", func(t *testing.T) {
		cli, err := client.New(userAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: user.SessionID,
		}))
		require.NoError(t, err)

		items := make([]iam.User, 0)
		total := new(int64)
		_, err = cli.List(&items, total)
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
	})
}

func TestAccountResetPassword(t *testing.T) {
	actor := accountSignupUser(t, "acct_reset_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)

	victim := accountSignupUser(t, "acct_reset_victim", "87654321")
	resetPass := "resetpass9"
	finalPass := "finalpass9"
	victimSessionBeforeReset := ""
	victimSessionAfterReset := ""

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		cli, err := client.New(resetpasswordAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Create(iam.ResetPasswordReq{
			UserID:      victim.UserID,
			NewPassword: resetPass,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "superuser required")
	})

	t.Run("victim_login_before_reset", func(t *testing.T) {
		victimSessionBeforeReset = accountLoginUser(t, &victim, victim.Password)
		require.NotEmpty(t, victimSessionBeforeReset)
		accountRequireUserSessionContains(t, victim.UserID, victimSessionBeforeReset)
	})

	t.Run("promote_actor_superuser", func(t *testing.T) {
		accountSetSuperuser(t, actor.Username, true)
	})

	t.Run("missing_target_returns_not_found", func(t *testing.T) {
		cli, err := client.New(resetpasswordAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Create(iam.ResetPasswordReq{
			UserID:      "missing-reset-password-target",
			NewPassword: resetPass,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
		require.Contains(t, err.Error(), "user not found")
	})

	t.Run("superuser_target_is_protected", func(t *testing.T) {
		protected := accountSignupUser(t, "acct_reset_protected", "12345678")
		accountSetSuperuser(t, protected.Username, true)

		cli, err := client.New(resetpasswordAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Create(iam.ResetPasswordReq{
			UserID:      protected.UserID,
			NewPassword: resetPass,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "superuser is protected")
	})

	t.Run("reset_success", func(t *testing.T) {
		cli, err := client.New(resetpasswordAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(iam.ResetPasswordReq{
			UserID:      victim.UserID,
			NewPassword: resetPass,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp *iam.ResetPasswordRsp) {
			t.Helper()
			require.NotEmpty(t, rsp.Msg)
		})
	})

	t.Run("victim_session_invalid_after_reset", func(t *testing.T) {
		accountRequireSessionNotFound(t, victimSessionBeforeReset)
		accountRequireUserSessionNotContains(t, victim.UserID, victimSessionBeforeReset)

		cli, err := client.New(userAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: victimSessionBeforeReset,
		}))
		require.NoError(t, err)

		items := make([]iam.User, 0)
		total := new(int64)
		_, err = cli.List(&items, total)
		require.Error(t, err)
		require.Contains(t, err.Error(), "401")
	})

	t.Run("victim_login_after_reset", func(t *testing.T) {
		victimSessionAfterReset = accountLoginUser(t, &victim, resetPass)
		require.NotEmpty(t, victimSessionAfterReset)
	})

	t.Run("must_change_password_blocks_list", func(t *testing.T) {
		cli, err := client.New(userAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: victimSessionAfterReset,
		}))
		require.NoError(t, err)

		items := make([]iam.User, 0)
		total := new(int64)
		_, err = cli.List(&items, total)
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
	})

	t.Run("victim_change_password", func(t *testing.T) {
		cli, err := client.New(changepasswordAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: victimSessionAfterReset,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(iam.ChangePasswordReq{
			OldPassword: resetPass,
			NewPassword: finalPass,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp *iam.ChangePasswordRsp) {
			t.Helper()
			require.NotEmpty(t, rsp.Msg)
		})
	})

	t.Run("victim_user_module_forbidden_after_change_password", func(t *testing.T) {
		cli, err := client.New(userAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: victimSessionAfterReset,
		}))
		require.NoError(t, err)

		items := make([]iam.User, 0)
		total := new(int64)
		_, err = cli.List(&items, total)
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
	})

	t.Run("demote_actor_superuser", func(t *testing.T) {
		accountSetSuperuser(t, actor.Username, false)
	})
}

func TestAccountStatus(t *testing.T) {
	actor := accountSignupUser(t, "acct_status_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)

	victim := accountSignupUser(t, "acct_status_victim", "acctpass11")
	victim.SessionID = accountLoginUser(t, &victim, victim.Password)
	accountRequireUserSessionContains(t, victim.UserID, victim.SessionID)

	victimSessionAfterEnable := ""

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Create(iam.AccountStatusReq{
			UserID: victim.UserID,
			Status: modeliamuser.UserStatusInactive,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "superuser required")
	})

	t.Run("promote_actor_superuser", func(t *testing.T) {
		accountSetSuperuser(t, actor.Username, true)
	})

	t.Run("missing_target_returns_not_found", func(t *testing.T) {
		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Create(iam.AccountStatusReq{
			UserID: "missing-account-status-target",
			Status: modeliamuser.UserStatusInactive,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
		require.Contains(t, err.Error(), "user not found")
	})

	t.Run("superuser_target_is_protected", func(t *testing.T) {
		protected := accountSignupUser(t, "acct_status_protected", "12345678")
		accountSetSuperuser(t, protected.Username, true)

		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Create(iam.AccountStatusReq{
			UserID: protected.UserID,
			Status: modeliamuser.UserStatusInactive,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "superuser is protected")
	})

	t.Run("disable_account", func(t *testing.T) {
		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(iam.AccountStatusReq{
			UserID: victim.UserID,
			Status: modeliamuser.UserStatusInactive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.AccountStatusRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "success")
		})
	})

	t.Run("session_invalid_after_disable", func(t *testing.T) {
		accountRequireSessionNotFound(t, victim.SessionID)
		accountRequireUserSessionNotContains(t, victim.UserID, victim.SessionID)

		cli, err := client.New(userAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: victim.SessionID,
		}))
		require.NoError(t, err)

		items := make([]iam.User, 0)
		total := new(int64)
		_, err = cli.List(&items, total)
		require.Error(t, err)
		require.Contains(t, err.Error(), "401")
	})

	t.Run("inactive_already_inactive_unchanged_still_ok", func(t *testing.T) {
		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(iam.AccountStatusReq{
			UserID: victim.UserID,
			Status: modeliamuser.UserStatusInactive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.AccountStatusRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "unchanged")
		})
	})

	t.Run("login_fails_when_inactive", func(t *testing.T) {
		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		_, err = cli.Create(iam.LoginReq{
			Username: victim.Username,
			Password: victim.Password,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), `"code":-1`)
		require.Contains(t, err.Error(), "disabled")
	})

	t.Run("enable_account", func(t *testing.T) {
		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(iam.AccountStatusReq{
			UserID: victim.UserID,
			Status: modeliamuser.UserStatusActive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.AccountStatusRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "success")
		})
	})

	t.Run("login_after_enable", func(t *testing.T) {
		victimSessionAfterEnable = accountLoginUser(t, &victim, victim.Password)
		require.NotEmpty(t, victimSessionAfterEnable)
		accountRequireUserSessionContains(t, victim.UserID, victimSessionAfterEnable)
	})

	t.Run("current_forbidden_when_db_inactive_but_redis_session_valid", func(t *testing.T) {
		victims := make([]*iam.User, 0)
		require.NoError(t, database.Database[*iam.User](context.Background()).WithLimit(1).WithQuery(&iam.User{Username: victim.Username}).List(&victims))
		require.Len(t, victims, 1)

		victimModel := victims[0]
		prevStatus := victimModel.Status
		victimModel.Status = modeliamuser.UserStatusInactive
		require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect("username", "status").Update(victimModel))
		t.Cleanup(func() {
			victimModel.Status = prevStatus
			require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect("username", "status").Update(victimModel))
			serviceiamsession.InvalidateUserStateCache(context.Background(), victim.UserID)
		})

		cli, err := client.New(currentAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: victimSessionAfterEnable,
		}))
		require.NoError(t, err)

		_, err = cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "account disabled")
		accountRequireSessionNotFound(t, victimSessionAfterEnable)
	})

	t.Run("current_forbidden_when_db_locked_but_redis_session_valid", func(t *testing.T) {
		sessionID := accountLoginUser(t, &victim, victim.Password)
		victims := make([]*iam.User, 0)
		require.NoError(t, database.Database[*iam.User](context.Background()).WithLimit(1).WithQuery(&iam.User{Username: victim.Username}).List(&victims))
		require.Len(t, victims, 1)

		victimModel := victims[0]
		prevStatus := victimModel.Status
		victimModel.Status = modeliamuser.UserStatusLocked
		require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect("username", "status").Update(victimModel))
		t.Cleanup(func() {
			victimModel.Status = prevStatus
			require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect("username", "status").Update(victimModel))
			serviceiamsession.InvalidateUserStateCache(context.Background(), victim.UserID)
		})

		cli, err := client.New(currentAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: sessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "account locked")
		accountRequireSessionNotFound(t, sessionID)
	})

	t.Run("invalid_status_rejected", func(t *testing.T) {
		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		_, err = cli.Create(iam.AccountStatusReq{
			UserID: victim.UserID,
			Status: modeliamuser.UserStatus("not-a-valid-status"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid")
	})

	t.Run("lock_account", func(t *testing.T) {
		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(iam.AccountStatusReq{
			UserID: victim.UserID,
			Status: modeliamuser.UserStatusLocked,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.AccountStatusRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "success")
		})
	})

	t.Run("session_invalid_after_lock", func(t *testing.T) {
		accountRequireSessionNotFound(t, victimSessionAfterEnable)
		accountRequireUserSessionNotContains(t, victim.UserID, victimSessionAfterEnable)

		cli, err := client.New(userAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: victimSessionAfterEnable,
		}))
		require.NoError(t, err)

		items := make([]iam.User, 0)
		total := new(int64)
		_, err = cli.List(&items, total)
		require.Error(t, err)
		require.Contains(t, err.Error(), "401")
	})

	t.Run("login_fails_when_locked", func(t *testing.T) {
		cli, err := client.New(loginAPI)
		require.NoError(t, err)

		_, err = cli.Create(iam.LoginReq{
			Username: victim.Username,
			Password: victim.Password,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), `"code":-1`)
		require.Contains(t, err.Error(), "locked")
	})

	t.Run("unlock_account", func(t *testing.T) {
		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(iam.AccountStatusReq{
			UserID: victim.UserID,
			Status: modeliamuser.UserStatusActive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.AccountStatusRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "success")
		})
	})

	t.Run("status_unchanged_idempotent", func(t *testing.T) {
		cli, err := client.New(accountstatusAPI, client.WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: actor.SessionID,
		}))
		require.NoError(t, err)

		resp, err := cli.Create(iam.AccountStatusReq{
			UserID: victim.UserID,
			Status: modeliamuser.UserStatusActive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.AccountStatusRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "unchanged")
		})
	})

	t.Run("demote_actor_superuser", func(t *testing.T) {
		accountSetSuperuser(t, actor.Username, false)
	})
}
