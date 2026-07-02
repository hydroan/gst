package iam_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/iam"
	"github.com/stretchr/testify/require"
)

func TestAdminUserList(t *testing.T) {
	rootSessionID := accountLoginRoot(t)
	user := accountSignupUserWithEmail(t, "admin_user_list", "12345678", "admin.user.list@example.com")
	fuzzyUser := accountSignupUserWithEmail(t, "admin_user_list_fuzzy_match", "12345678", "admin.user.list.fuzzy@example.com")
	actor := accountSignupUser(t, "admin_user_list_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)

	t.Run("list_users", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, adminUsersAPI(), rootSessionID)

		items := make([]iam.AdminUserView, 0)
		total := new(int64)
		_, err := cli.List(&items, total)
		require.NoError(t, err)
		require.Positive(t, *total)

		view := requireAdminUserView(t, items, user.UserID)
		require.Equal(t, user.UserID, view.ID)
		require.Equal(t, user.Username, view.Username)
		require.Equal(t, "admin.user.list@example.com", view.Email)
		require.Equal(t, modeliamuser.UserStatusActive, view.Status)
		require.NotZero(t, view.CreatedAt)
	})

	t.Run("filter_by_username", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, adminUsersAPI()+"?username=fuzzy_match", rootSessionID)

		items := make([]iam.AdminUserView, 0)
		total := new(int64)
		_, err := cli.List(&items, total)
		require.NoError(t, err)
		require.EqualValues(t, 1, *total)
		require.Len(t, items, 1)
		require.Equal(t, fuzzyUser.UserID, items[0].ID)
		require.Equal(t, fuzzyUser.Username, items[0].Username)
	})

	t.Run("forbidden_without_admin_permission", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, adminUsersAPI(), actor.SessionID)

		items := make([]iam.AdminUserView, 0)
		total := new(int64)
		_, err := cli.List(&items, total)
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "permission denied")
	})
}

func TestAdminUserGet(t *testing.T) {
	rootSessionID := accountLoginRoot(t)
	user := accountSignupUserWithEmail(t, "admin_user_get", "12345678", "admin.user.get@example.com")
	actor := accountSignupUser(t, "admin_user_get_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)

	t.Run("get_user", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, adminUsersAPI(), rootSessionID)

		got := new(iam.AdminUserGetRsp)
		_, err := cli.Get(user.UserID, got)
		require.NoError(t, err)
		require.Equal(t, user.UserID, got.User.ID)
		require.Equal(t, user.Username, got.User.Username)
		require.Equal(t, "admin.user.get@example.com", got.User.Email)
		require.Equal(t, modeliamuser.UserStatusActive, got.User.Status)
		require.NotZero(t, got.User.CreatedAt)
	})

	t.Run("missing_target_returns_not_found", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, adminUsersAPI(), rootSessionID)

		got := new(iam.AdminUserGetRsp)
		_, err := cli.Get("missing-admin-user-get-target", got)
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
		require.Contains(t, err.Error(), "user not found")
	})

	t.Run("forbidden_without_admin_permission", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, adminUsersAPI(), actor.SessionID)

		got := new(iam.AdminUserGetRsp)
		_, err := cli.Get(user.UserID, got)
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "permission denied")
	})
}

func TestUserStatusPatch(t *testing.T) {
	actor := accountSignupUser(t, "user_status_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)
	rootSessionID := accountLoginRoot(t)

	victim := accountSignupUser(t, "user_status_victim", "acctpass11")
	victim.SessionID = accountLoginUser(t, &victim, victim.Password)
	accountRequireUserSessionContains(t, victim.UserID, victim.SessionID)

	victimSessionAfterEnable := ""

	t.Run("forbidden_without_admin_permission", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, userStatusAPI(victim.UserID), actor.SessionID)

		_, err := cli.Request(http.MethodPatch, iam.UserStatusPatchReq{
			Status: modeliamuser.UserStatusInactive,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "permission denied")
	})

	t.Run("missing_target_returns_not_found", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, userStatusAPI("missing-user-status-target"), rootSessionID)

		_, err := cli.Request(http.MethodPatch, iam.UserStatusPatchReq{
			Status: modeliamuser.UserStatusInactive,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
		require.Contains(t, err.Error(), "user not found")
	})

	t.Run("disable_user", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, userStatusAPI(victim.UserID), rootSessionID)

		resp, err := cli.Request(http.MethodPatch, iam.UserStatusPatchReq{
			Status: modeliamuser.UserStatusInactive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.UserStatusPatchRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "success")
		})
	})

	t.Run("session_invalid_after_disable", func(t *testing.T) {
		accountRequireSessionNotFound(t, victim.SessionID)
		accountRequireUserSessionNotContains(t, victim.UserID, victim.SessionID)

		cli := accountNewAuthenticatedClient(t, currentAPI, victim.SessionID)

		_, err := cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "401")
	})

	t.Run("inactive_already_inactive_unchanged_still_ok", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, userStatusAPI(victim.UserID), rootSessionID)

		resp, err := cli.Request(http.MethodPatch, iam.UserStatusPatchReq{
			Status: modeliamuser.UserStatusInactive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.UserStatusPatchRsp) {
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

	t.Run("enable_user", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, userStatusAPI(victim.UserID), rootSessionID)

		resp, err := cli.Request(http.MethodPatch, iam.UserStatusPatchReq{
			Status: modeliamuser.UserStatusActive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.UserStatusPatchRsp) {
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
		victimModel := userLoadByUsername(t, victim.Username)
		prevStatus := victimModel.Status
		victimModel.Status = modeliamuser.UserStatusInactive
		require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect("username", "status").Update(victimModel))
		t.Cleanup(func() {
			victimModel.Status = prevStatus
			require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect("username", "status").Update(victimModel))
			serviceiamsession.InvalidateUserStateCache(context.Background(), victim.UserID)
		})

		cli := accountNewAuthenticatedClient(t, currentAPI, victimSessionAfterEnable)

		_, err := cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "account disabled")
		accountRequireSessionNotFound(t, victimSessionAfterEnable)
	})

	t.Run("current_forbidden_when_db_locked_but_redis_session_valid", func(t *testing.T) {
		sessionID := accountLoginUser(t, &victim, victim.Password)
		victimModel := userLoadByUsername(t, victim.Username)
		prevStatus := victimModel.Status
		victimModel.Status = modeliamuser.UserStatusLocked
		require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect("username", "status").Update(victimModel))
		t.Cleanup(func() {
			victimModel.Status = prevStatus
			require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect("username", "status").Update(victimModel))
			serviceiamsession.InvalidateUserStateCache(context.Background(), victim.UserID)
		})

		cli := accountNewAuthenticatedClient(t, currentAPI, sessionID)

		_, err := cli.Request(http.MethodGet, new(struct{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "403")
		require.Contains(t, err.Error(), "account locked")
		accountRequireSessionNotFound(t, sessionID)
	})

	t.Run("invalid_status_rejected", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, userStatusAPI(victim.UserID), rootSessionID)

		_, err := cli.Request(http.MethodPatch, iam.UserStatusPatchReq{
			Status: modeliamuser.UserStatus("not-a-valid-status"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid")
	})

	t.Run("lock_user", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, userStatusAPI(victim.UserID), rootSessionID)

		resp, err := cli.Request(http.MethodPatch, iam.UserStatusPatchReq{
			Status: modeliamuser.UserStatusLocked,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.UserStatusPatchRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "success")
		})
	})

	t.Run("session_invalid_after_lock", func(t *testing.T) {
		accountRequireSessionNotFound(t, victimSessionAfterEnable)
		accountRequireUserSessionNotContains(t, victim.UserID, victimSessionAfterEnable)

		cli := accountNewAuthenticatedClient(t, currentAPI, victimSessionAfterEnable)

		_, err := cli.Request(http.MethodGet, new(struct{}))
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

	t.Run("unlock_user", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, userStatusAPI(victim.UserID), rootSessionID)

		resp, err := cli.Request(http.MethodPatch, iam.UserStatusPatchReq{
			Status: modeliamuser.UserStatusActive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.UserStatusPatchRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "success")
		})
	})

	t.Run("status_unchanged_idempotent", func(t *testing.T) {
		cli := accountNewAuthenticatedClient(t, userStatusAPI(victim.UserID), rootSessionID)

		resp, err := cli.Request(http.MethodPatch, iam.UserStatusPatchReq{
			Status: modeliamuser.UserStatusActive,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.UserStatusPatchRsp) {
			t.Helper()
			require.Contains(t, rsp.Msg, "unchanged")
		})
	})
}

func userLoadByUsername(t *testing.T, username string) *iam.User {
	t.Helper()

	users := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](context.Background()).WithLimit(1).WithQuery(&iam.User{Username: username}).List(&users))
	require.Len(t, users, 1)
	return users[0]
}

func adminUsersAPI() string {
	return testutil.URL(port, "/api/iam/admin/users")
}

func requireAdminUserView(t *testing.T, items []iam.AdminUserView, userID string) iam.AdminUserView {
	t.Helper()

	for i := range items {
		if items[i].ID == userID {
			return items[i]
		}
	}

	require.Failf(t, "admin user view not found", "user_id=%s", userID)
	return iam.AdminUserView{}
}
