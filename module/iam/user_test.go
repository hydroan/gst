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
