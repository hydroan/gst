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
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

const adminUsersPath = "/api/iam/admin/users"

// Column references for the user fixture writes in the user and session
// tests; module test code carries no generated Cols vars.
var (
	colUsername   = types.NewColumn[string]("username")
	colUserStatus = types.NewColumn[modeliamuser.UserStatus]("status")
)

func TestAdminUserList(t *testing.T) {
	rootSessionID := accountLoginRoot(t)
	user := accountSignupUserWithEmail(t, "admin_user_list", "12345678", "admin.user.list@example.com")
	fuzzyUser := accountSignupUserWithEmail(t, "admin_user_list_fuzzy_match", "12345678", "admin.user.list.fuzzy@example.com")
	actor := accountSignupUser(t, "admin_user_list_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)

	t.Run("list_users", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		list, err := cli.Get[client.ListResult[iam.AdminUserView]](adminUsersPath)
		require.NoError(t, err)
		require.Positive(t, list.Total)

		view := requireAdminUserView(t, list.Items, user.UserID)
		require.Equal(t, user.UserID, view.ID)
		require.Equal(t, user.Username, view.Username)
		require.Equal(t, "admin.user.list@example.com", view.Email)
		require.Equal(t, modeliamuser.UserStatusActive, view.Status)
		require.NotZero(t, view.CreatedAt)
	})

	// A bare field name is an exact match, and a substring is asked for with the
	// like operator. This endpoint used to read the bare name as a substring,
	// which is the one place in the framework where it meant that.
	t.Run("filters_by_exact_username", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		list, err := cli.Get[client.ListResult[iam.AdminUserView]](adminUsersPath,
			client.WithQuery("username", fuzzyUser.Username))
		require.NoError(t, err)
		require.Equal(t, 1, list.Total)
		require.Len(t, list.Items, 1)
		require.Equal(t, fuzzyUser.UserID, list.Items[0].ID)

		// The substring alone matches nothing, because it is not the username.
		list, err = cli.Get[client.ListResult[iam.AdminUserView]](adminUsersPath,
			client.WithQuery("username", "fuzzy_match"))
		require.NoError(t, err)
		require.Zero(t, list.Total)
	})

	t.Run("filters_by_username_substring", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		list, err := cli.Get[client.ListResult[iam.AdminUserView]](adminUsersPath,
			client.WithQuery("username[like]", "fuzzy_match"))
		require.NoError(t, err)
		require.Equal(t, 1, list.Total)
		require.Len(t, list.Items, 1)
		require.Equal(t, fuzzyUser.UserID, list.Items[0].ID)
	})

	// Ordering is one of the general list parameters this endpoint answers to
	// now that it parses the request the way every other list does; it used to
	// understand a username filter and paging and nothing else.
	t.Run("orders_by_a_requested_column", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		descending, err := cli.Get[client.ListResult[iam.AdminUserView]](adminUsersPath,
			client.WithQuery("username[like]", "admin_user_list"),
			client.WithQuery("_sort_by", "username desc"))
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(descending.Items), 2)

		ascending, err := cli.Get[client.ListResult[iam.AdminUserView]](adminUsersPath,
			client.WithQuery("username[like]", "admin_user_list"),
			client.WithQuery("_sort_by", "username asc"))
		require.NoError(t, err)
		require.Len(t, ascending.Items, len(descending.Items))

		require.Equal(t, descending.Items[0].Username, ascending.Items[len(ascending.Items)-1].Username)
		require.Greater(t, descending.Items[0].Username, descending.Items[len(descending.Items)-1].Username)
	})

	t.Run("rejects_a_filter_naming_no_column", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		// A mistyped filter is refused rather than ignored: dropping it would
		// silently return more rows than the caller asked for.
		_, err := cli.Get[client.ListResult[iam.AdminUserView]](adminUsersPath,
			client.WithQuery("nosuchfield[like]", "x"))
		testutil.RequireError(t, err, http.StatusBadRequest)
	})

	t.Run("forbidden_without_admin_permission", func(t *testing.T) {
		cli := accountSessionClient(t, actor.SessionID)

		_, err := cli.Get[client.ListResult[iam.AdminUserView]](adminUsersPath)
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")
	})
}

func TestAdminUserGet(t *testing.T) {
	rootSessionID := accountLoginRoot(t)
	user := accountSignupUserWithEmail(t, "admin_user_get", "12345678", "admin.user.get@example.com")
	actor := accountSignupUser(t, "admin_user_get_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)

	t.Run("get_user", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		got, err := cli.Get[iam.AdminUserGetRsp](adminUsersPath + "/" + user.UserID)
		require.NoError(t, err)
		require.Equal(t, user.UserID, got.User.ID)
		require.Equal(t, user.Username, got.User.Username)
		require.Equal(t, "admin.user.get@example.com", got.User.Email)
		require.Equal(t, modeliamuser.UserStatusActive, got.User.Status)
		require.NotZero(t, got.User.CreatedAt)
	})

	t.Run("missing_target_returns_not_found", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Get[iam.AdminUserGetRsp](adminUsersPath + "/missing-admin-user-get-target")
		testutil.RequireError(t, err, http.StatusNotFound, "user not found")
	})

	t.Run("forbidden_without_admin_permission", func(t *testing.T) {
		cli := accountSessionClient(t, actor.SessionID)

		_, err := cli.Get[iam.AdminUserGetRsp](adminUsersPath + "/" + user.UserID)
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")
	})
}

func TestAdminUserPatch(t *testing.T) {
	actor := accountSignupUser(t, "user_status_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)
	rootSessionID := accountLoginRoot(t)

	victim := accountSignupUser(t, "user_status_victim", "acctpass11")
	victim.SessionID = accountLoginUser(t, &victim, victim.Password)
	accountRequireUserSessionContains(t, victim.UserID, victim.SessionID)

	victimSessionAfterEnable := ""

	t.Run("forbidden_without_admin_permission", func(t *testing.T) {
		cli := accountSessionClient(t, actor.SessionID)

		_, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusInactive)})
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")
	})

	t.Run("missing_target_returns_not_found", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath("missing-user-status-target"), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusInactive)})
		testutil.RequireError(t, err, http.StatusNotFound, "user not found")
	})

	t.Run("disable_user", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		rsp, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusInactive)})
		require.NoError(t, err)
		require.Equal(t, modeliamuser.UserStatusInactive, rsp.User.Status)
	})

	t.Run("session_invalid_after_disable", func(t *testing.T) {
		accountRequireSessionNotFound(t, victim.SessionID)
		accountRequireUserSessionNotContains(t, victim.UserID, victim.SessionID)

		cli := accountSessionClient(t, victim.SessionID)

		_, err := cli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})

	t.Run("inactive_already_inactive_unchanged_still_ok", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		rsp, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusInactive)})
		require.NoError(t, err)
		require.Equal(t, modeliamuser.UserStatusInactive, rsp.User.Status)
	})

	t.Run("login_fails_when_inactive", func(t *testing.T) {
		cli, err := client.New(baseURL)
		require.NoError(t, err)

		_, err = cli.Post[iam.LoginRsp](loginPath, iam.LoginReq{
			Username: victim.Username,
			Password: victim.Password,
		})
		respErr := testutil.RequireError(t, err, http.StatusForbidden, "disabled")
		require.Equal(t, -1, respErr.Code)
	})

	t.Run("enable_user", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		rsp, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusActive)})
		require.NoError(t, err)
		require.Equal(t, modeliamuser.UserStatusActive, rsp.User.Status)
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
		require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect(colUsername, colUserStatus).Update(victimModel))
		t.Cleanup(func() {
			victimModel.Status = prevStatus
			require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect(colUsername, colUserStatus).Update(victimModel))
			serviceiamsession.Store.DropUserState(context.Background(), victim.UserID)
		})

		cli := accountSessionClient(t, victimSessionAfterEnable)

		_, err := cli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusForbidden, "account disabled")
		accountRequireSessionNotFound(t, victimSessionAfterEnable)
	})

	t.Run("current_forbidden_when_db_locked_but_redis_session_valid", func(t *testing.T) {
		sessionID := accountLoginUser(t, &victim, victim.Password)
		victimModel := userLoadByUsername(t, victim.Username)
		prevStatus := victimModel.Status
		victimModel.Status = modeliamuser.UserStatusLocked
		require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect(colUsername, colUserStatus).Update(victimModel))
		t.Cleanup(func() {
			victimModel.Status = prevStatus
			require.NoError(t, database.Database[*iam.User](context.Background()).WithoutHook().WithSelect(colUsername, colUserStatus).Update(victimModel))
			serviceiamsession.Store.DropUserState(context.Background(), victim.UserID)
		})

		cli := accountSessionClient(t, sessionID)

		_, err := cli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusForbidden, "account locked")
		accountRequireSessionNotFound(t, sessionID)
	})

	t.Run("invalid_status_rejected", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{
			Status: new(modeliamuser.UserStatus("not-a-valid-status")),
		})
		testutil.RequireError(t, err, http.StatusBadRequest, "invalid")
	})

	t.Run("lock_user", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		rsp, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusLocked)})
		require.NoError(t, err)
		require.Equal(t, modeliamuser.UserStatusLocked, rsp.User.Status)
	})

	t.Run("session_invalid_after_lock", func(t *testing.T) {
		accountRequireSessionNotFound(t, victimSessionAfterEnable)
		accountRequireUserSessionNotContains(t, victim.UserID, victimSessionAfterEnable)

		cli := accountSessionClient(t, victimSessionAfterEnable)

		_, err := cli.Get[iam.CurrentGetRsp](currentPath)
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})

	t.Run("login_fails_when_locked", func(t *testing.T) {
		cli, err := client.New(baseURL)
		require.NoError(t, err)

		_, err = cli.Post[iam.LoginRsp](loginPath, iam.LoginReq{
			Username: victim.Username,
			Password: victim.Password,
		})
		respErr := testutil.RequireError(t, err, http.StatusForbidden, "locked")
		require.Equal(t, -1, respErr.Code)
	})

	t.Run("unlock_user", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		rsp, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusActive)})
		require.NoError(t, err)
		require.Equal(t, modeliamuser.UserStatusActive, rsp.User.Status)
	})

	t.Run("status_unchanged_idempotent", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		rsp, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(victim.UserID), iam.AdminUserPatchReq{Status: new(modeliamuser.UserStatusActive)})
		require.NoError(t, err)
		require.Equal(t, modeliamuser.UserStatusActive, rsp.User.Status)
	})
}

func userLoadByUsername(t *testing.T, username string) *iam.User {
	t.Helper()

	users := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](context.Background()).WithQuery(&iam.User{Username: username}).List(&users))
	require.Len(t, users, 1)
	return users[0]
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

// TestAdminUserCreate covers how an account comes into being in a deployment
// that does not offer public signup, which is most of them.
func TestAdminUserCreate(t *testing.T) {
	rootSessionID := accountLoginRoot(t)
	actor := accountSignupUser(t, "admin_user_create_actor", "12345678")
	actor.SessionID = accountLoginUser(t, &actor, actor.Password)

	t.Run("creates_a_user_that_can_sign_in", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)
		username := "admin_user_create_target"
		password := "created-pass9"

		created, err := cli.Post[iam.AdminUserCreateRsp](adminUsersPath, iam.AdminUserCreateReq{
			Username: username,
			Password: password,
			Email:    "admin.user.create@example.com",
			// Opted out so the account is usable straight away; the default is
			// covered by the subtest below.
			MustChangePassword: new(false),
		})
		require.NoError(t, err)
		require.NotEmpty(t, created.User.ID)
		require.Equal(t, username, created.User.Username)
		require.Equal(t, "admin.user.create@example.com", created.User.Email)
		require.Equal(t, modeliamuser.UserStatusActive, created.User.Status)
		require.False(t, created.User.MustChangePassword)

		// The credential is the point of creating the account, so the assertion
		// is that it actually signs in rather than that a row exists.
		newUser := accountTestUser{Username: username, Password: password, UserID: created.User.ID}
		require.NotEmpty(t, accountLoginUser(t, &newUser, password))
	})

	t.Run("defaults_to_requiring_a_password_change", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		created, err := cli.Post[iam.AdminUserCreateRsp](adminUsersPath, iam.AdminUserCreateReq{
			Username: "admin_user_create_must_change",
			Password: "created-pass9",
		})
		require.NoError(t, err)
		require.True(t, created.User.MustChangePassword,
			"a password someone else chose is a password two people know")
	})

	t.Run("rejects_a_duplicate_username", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Post[iam.AdminUserCreateRsp](adminUsersPath, iam.AdminUserCreateReq{
			Username: "admin_user_create_target",
			Password: "created-pass9",
		})
		testutil.RequireError(t, err, http.StatusConflict)
	})

	t.Run("rejects_a_password_the_policy_refuses", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Post[iam.AdminUserCreateRsp](adminUsersPath, iam.AdminUserCreateReq{
			Username: "admin_user_create_short_password",
			Password: "short",
		})
		testutil.RequireError(t, err, http.StatusBadRequest)

		// Nothing is left behind by the rejected attempt: the user row and the
		// credential are created in one transaction.
		list, err := cli.Get[client.ListResult[iam.AdminUserView]](adminUsersPath,
			client.WithQuery("username", "admin_user_create_short_password"))
		require.NoError(t, err)
		require.Zero(t, list.Total)
	})

	t.Run("forbidden_without_admin_permission", func(t *testing.T) {
		cli := accountSessionClient(t, actor.SessionID)

		_, err := cli.Post[iam.AdminUserCreateRsp](adminUsersPath, iam.AdminUserCreateReq{
			Username: "admin_user_create_forbidden",
			Password: "created-pass9",
		})
		testutil.RequireError(t, err, http.StatusForbidden, "permission denied")
	})
}

// TestAdminUserPatchUsername covers the field the patch route gained when the
// per-field status route was folded into it.
func TestAdminUserPatchUsername(t *testing.T) {
	rootSessionID := accountLoginRoot(t)
	target := accountSignupUser(t, "admin_user_rename", "12345678")

	t.Run("renames_a_user", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)
		renamed := "admin_user_renamed"

		rsp, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(target.UserID), iam.AdminUserPatchReq{
			Username: &renamed,
		})
		require.NoError(t, err)
		require.Equal(t, renamed, rsp.User.Username)

		// The status was not named by the request and is left as it was.
		require.Equal(t, modeliamuser.UserStatusActive, rsp.User.Status)
	})

	t.Run("rejects_a_request_naming_no_field", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)

		_, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(target.UserID), iam.AdminUserPatchReq{})
		testutil.RequireError(t, err, http.StatusBadRequest)
	})

	t.Run("rejects_an_empty_username", func(t *testing.T) {
		cli := accountSessionClient(t, rootSessionID)
		empty := "   "

		_, err := cli.Patch[iam.AdminUserPatchRsp](adminUserPath(target.UserID), iam.AdminUserPatchReq{
			Username: &empty,
		})
		testutil.RequireError(t, err, http.StatusBadRequest)
	})
}
