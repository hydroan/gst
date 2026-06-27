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
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

const testSuccessCode = 0

type userTestAccount struct {
	UserID    string
	Username  string
	Password  string
	SessionID string
}

type userBatchRsp struct {
	Items   []*iam.User `json:"items"`
	Summary struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	} `json:"summary"`
}

func TestUserList(t *testing.T) {
	actor := userSignupUser(t, "user_list_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	viewer := userSignupUser(t, "user_list_viewer", "12345678")

	cli := userNewClient(t, actor.SessionID)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		items := make([]*iam.User, 0)
		total := new(int64)
		_, err := cli.List(&items, total)
		userRequireForbidden(t, err)
	})

	t.Run("promote_actor_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)
	})

	t.Run("list_users", func(t *testing.T) {
		items := make([]*iam.User, 0)
		total := new(int64)
		resp, err := cli.List(&items, total)
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp ListResponse[*iam.User]) {
			t.Helper()
			actorItem := userFindByUsername(rsp.Items, actor.Username)
			require.NotNil(t, actorItem)
			require.Equal(t, actor.Username, actorItem.Username)
			require.True(t, actorItem.IsSuperuser != nil && *actorItem.IsSuperuser)
			require.Empty(t, actorItem.Password)
			require.Empty(t, actorItem.PasswordHash)
			require.Empty(t, actorItem.Salt)

			viewerItem := userFindByUsername(rsp.Items, viewer.Username)
			require.NotNil(t, viewerItem)
			require.Equal(t, modeliamuser.UserStatusActive, viewerItem.Status)
			require.Equal(t, modeliamuser.UserTypeRegular, viewerItem.Type)
			require.Empty(t, viewerItem.Password)
			require.Empty(t, viewerItem.PasswordHash)
			require.Empty(t, viewerItem.Salt)
		})
	})
}

func TestUserCreate(t *testing.T) {
	actor := userSignupUser(t, "user_create_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	cli := userNewClient(t, actor.SessionID)
	targetUsername := fmt.Sprintf("user_create_target_%d", time.Now().UnixNano())
	targetDisplayName := "Created By Superuser"
	targetEmail := targetUsername + "@example.com"
	superuserEnabled := true
	superUsername := fmt.Sprintf("user_create_super_%d", time.Now().UnixNano())

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Create(iam.User{
			Username: targetUsername,
			Password: "example-UserCreate-local-01",
		})
		userRequireForbidden(t, err)
	})

	t.Run("promote_actor_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)
	})

	t.Run("create_user", func(t *testing.T) {
		resp, err := cli.Create(iam.User{
			Username:    targetUsername,
			Password:    "example-UserCreate-local-01",
			DisplayName: &targetDisplayName,
			Email:       &targetEmail,
		})
		require.NoError(t, err)

		t.Cleanup(func() {
			userCleanupUser(t, targetUsername)
		})

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.User) {
			t.Helper()
			require.NotEmpty(t, rsp.ID)
			require.Equal(t, targetUsername, rsp.Username)
			require.Empty(t, rsp.Password)
			require.Empty(t, rsp.PasswordHash)
			require.NotNil(t, rsp.DisplayName)
			require.Equal(t, targetDisplayName, *rsp.DisplayName)
			require.NotNil(t, rsp.Email)
			require.Equal(t, targetEmail, *rsp.Email)
		})

		stored := userLoadByUsername(t, targetUsername)
		require.NotNil(t, stored.DisplayName)
		require.Equal(t, targetDisplayName, *stored.DisplayName)
		require.NotNil(t, stored.Email)
		require.Equal(t, targetEmail, *stored.Email)
	})

	t.Run("create_superuser_forbidden", func(t *testing.T) {
		_, err := cli.Create(iam.User{
			Username:    superUsername,
			Password:    "example-UserCreate-local-02",
			IsSuperuser: &superuserEnabled,
		})
		userRequireForbidden(t, err)
		userRequireMissingByUsername(t, superUsername)
	})

	t.Run("root_username_without_root_user_id_cannot_create_superuser", func(t *testing.T) {
		spoofedRootCli := userNewSpoofedRootClient(t, "user_create_spoofed_root")
		spoofedRootCreatedUsername := fmt.Sprintf("user_create_spoofed_root_created_%d", time.Now().UnixNano())

		_, err := spoofedRootCli.Create(iam.User{
			Username:    spoofedRootCreatedUsername,
			Password:    "example-UserCreate-local-03",
			IsSuperuser: &superuserEnabled,
		})
		userRequireForbidden(t, err)
		userRequireMissingByUsername(t, spoofedRootCreatedUsername)
	})
}

func TestUserCreateMany(t *testing.T) {
	actor := userSignupUser(t, "user_create_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	cli := userNewClient(t, actor.SessionID)
	username1 := fmt.Sprintf("user_create_many_target1_%d", time.Now().UnixNano())
	username2 := fmt.Sprintf("user_create_many_target2_%d", time.Now().UnixNano())
	displayName1 := "Create Many User 1"
	displayName2 := "Create Many User 2"
	email1 := username1 + "@example.com"
	email2 := username2 + "@example.com"
	superuserEnabled := true
	superUsername := fmt.Sprintf("user_create_many_super_%d", time.Now().UnixNano())

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.CreateMany([]iam.User{
			{
				Username: username1,
				Password: "example-UserCreateMany-local-01",
			},
		})
		userRequireForbidden(t, err)
	})

	t.Run("promote_actor_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)
	})

	t.Run("create_many_users", func(t *testing.T) {
		resp, err := cli.CreateMany([]iam.User{
			{
				Username:    username1,
				Password:    "example-UserCreateMany-local-01",
				DisplayName: &displayName1,
				Email:       &email1,
			},
			{
				Username:    username2,
				Password:    "example-UserCreateMany-local-02",
				DisplayName: &displayName2,
				Email:       &email2,
			},
		})
		require.NoError(t, err)

		t.Cleanup(func() {
			userCleanupUser(t, username1)
			userCleanupUser(t, username2)
		})

		testutil.TestResp(t, resp, func(t *testing.T, rsp userBatchRsp) {
			t.Helper()
			require.Len(t, rsp.Items, 2)
			require.Equal(t, 2, rsp.Summary.Total)
			require.Equal(t, 2, rsp.Summary.Succeeded)
			require.Equal(t, 0, rsp.Summary.Failed)
			for _, item := range rsp.Items {
				require.NotNil(t, item)
				require.NotEmpty(t, item.ID)
				require.Empty(t, item.Password)
				require.Empty(t, item.PasswordHash)
				require.Empty(t, item.Salt)
			}
		})

		user1 := userLoadByUsername(t, username1)
		require.NotNil(t, user1.DisplayName)
		require.Equal(t, displayName1, *user1.DisplayName)
		require.NotNil(t, user1.Email)
		require.Equal(t, email1, *user1.Email)

		user2 := userLoadByUsername(t, username2)
		require.NotNil(t, user2.DisplayName)
		require.Equal(t, displayName2, *user2.DisplayName)
		require.NotNil(t, user2.Email)
		require.Equal(t, email2, *user2.Email)
	})

	t.Run("create_many_superuser_forbidden", func(t *testing.T) {
		_, err := cli.CreateMany([]iam.User{
			{
				Username:    superUsername,
				Password:    "example-UserCreateMany-local-03",
				IsSuperuser: &superuserEnabled,
			},
		})
		userRequireForbidden(t, err)
		userRequireMissingByUsername(t, superUsername)
	})
}

func TestUserGet(t *testing.T) {
	actor := userSignupUser(t, "user_get_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	victim := userSignupUser(t, "user_get_target", "example-UserGet-local-01")

	cli := userNewClient(t, actor.SessionID)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Get(victim.UserID, new(iam.User))
		userRequireForbidden(t, err)
	})

	t.Run("promote_actor_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)
	})

	t.Run("get_user", func(t *testing.T) {
		got := new(iam.User)
		resp, err := cli.Get(victim.UserID, got)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, testSuccessCode, resp.Code)
		require.Equal(t, victim.UserID, got.ID)
		require.Equal(t, victim.Username, got.Username)
		require.Equal(t, modeliamuser.UserStatusActive, got.Status)
		require.Equal(t, modeliamuser.UserTypeRegular, got.Type)
		require.Empty(t, got.Password)
		require.Empty(t, got.PasswordHash)
		require.Empty(t, got.Salt)
	})

	t.Run("get_superuser_forbidden", func(t *testing.T) {
		superVictim := userSignupUser(t, "user_get_super_target", "example-UserGet-local-02")
		userSetSuperuser(t, superVictim.Username, true)

		_, err := cli.Get(superVictim.UserID, new(iam.User))
		userRequireForbidden(t, err)
	})

	t.Run("root_username_without_root_user_id_cannot_get_superuser", func(t *testing.T) {
		spoofedRootCli := userNewSpoofedRootClient(t, "user_get_spoofed_root")
		superVictim := userSignupUser(t, "user_get_spoofed_root_target", "example-UserGet-local-03")
		userSetSuperuser(t, superVictim.Username, true)

		_, err := spoofedRootCli.Get(superVictim.UserID, new(iam.User))
		userRequireForbidden(t, err)
	})

	t.Run("get_user_not_found", func(t *testing.T) {
		_, err := cli.Get("missing-user-id", new(iam.User))
		userRequireNotFound(t, err)
	})
}

func TestUserPatch(t *testing.T) {
	actor := userSignupUser(t, "user_patch_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	victim := userSignupUser(t, "user_patch_target", "example-UserPatch-local-01")
	cli := userNewClient(t, actor.SessionID)
	patchedDisplayName := "Patched By PATCH"

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		_, err := cli.Patch(victim.UserID, modeliamuser.UserPatchReq{
			DisplayName: &patchedDisplayName,
		})
		userRequireForbidden(t, err)
	})

	t.Run("promote_actor_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)
	})

	t.Run("patch_user", func(t *testing.T) {
		resp, err := cli.Patch(victim.UserID, modeliamuser.UserPatchReq{
			DisplayName: &patchedDisplayName,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, testSuccessCode, resp.Code)
		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.User) {
			t.Helper()
			require.Equal(t, victim.UserID, rsp.ID)
			require.Empty(t, rsp.Password)
			require.Empty(t, rsp.PasswordHash)
			require.Empty(t, rsp.Salt)
		})

		stored := userLoadByID(t, victim.UserID)
		require.NotNil(t, stored.DisplayName)
		require.Equal(t, patchedDisplayName, *stored.DisplayName)
	})

	t.Run("patch_user_not_found", func(t *testing.T) {
		_, err := cli.Patch("missing-user-id", modeliamuser.UserPatchReq{
			DisplayName: &patchedDisplayName,
		})
		userRequireNotFound(t, err)
	})

	t.Run("patch_superuser_forbidden", func(t *testing.T) {
		superVictim := userSignupUser(t, "user_patch_super_target", "example-UserPatch-local-02")
		userSetSuperuser(t, superVictim.Username, true)

		_, err := cli.Patch(superVictim.UserID, modeliamuser.UserPatchReq{
			DisplayName: &patchedDisplayName,
		})
		userRequireForbidden(t, err)
	})

	t.Run("root_username_without_root_user_id_cannot_patch_superuser", func(t *testing.T) {
		spoofedRootCli := userNewSpoofedRootClient(t, "user_patch_spoofed_root")
		superVictim := userSignupUser(t, "user_patch_spoofed_root_target", "example-UserPatch-local-03")
		userSetSuperuser(t, superVictim.Username, true)
		spoofedRootPatchedDisplayName := "spoofed-root-updated-superuser"

		_, err := spoofedRootCli.Patch(superVictim.UserID, modeliamuser.UserPatchReq{
			DisplayName: &spoofedRootPatchedDisplayName,
		})
		userRequireForbidden(t, err)
	})

	t.Run("patch_sensitive_fields_rejected", func(t *testing.T) {
		cases := []struct {
			name    string
			payload func(t *testing.T, target userTestAccount) string
		}{
			{
				name: "username",
				payload: func(t *testing.T, target userTestAccount) string {
					t.Helper()
					username := fmt.Sprintf("user_patch_blocked_username_%d", time.Now().UnixNano())
					t.Cleanup(func() {
						userCleanupUser(t, username)
					})
					return fmt.Sprintf(`{"username":%q}`, username)
				},
			},
			{
				name: "status",
				payload: func(t *testing.T, target userTestAccount) string {
					t.Helper()
					return fmt.Sprintf(`{"status":%q}`, modeliamuser.UserStatusInactive)
				},
			},
			{
				name: "is_superuser",
				payload: func(t *testing.T, target userTestAccount) string {
					t.Helper()
					return `{"is_superuser":true}`
				},
			},
			{
				name: "password",
				payload: func(t *testing.T, target userTestAccount) string {
					t.Helper()
					return `{"password":"blocked-password-123"}`
				},
			},
			{
				name: "unknown_field",
				payload: func(t *testing.T, target userTestAccount) string {
					t.Helper()
					return `{"unexpected_admin_field":true}`
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				target := userSignupUser(t, "user_patch_sensitive_"+tc.name, "example-UserPatchSensitive-local-01")
				before := userLoadByID(t, target.UserID)

				_, err := cli.Patch(target.UserID, []byte(tc.payload(t, target)))
				userRequirePatchRejected(t, err)

				stored := userLoadByID(t, target.UserID)
				require.Equal(t, before.Username, stored.Username)
				require.Equal(t, before.Status, stored.Status)
				require.Equal(t, before.Type, stored.Type)
				require.Equal(t, before.PasswordHash, stored.PasswordHash)
				require.Equal(t, before.IsSuperuser, stored.IsSuperuser)
				require.Equal(t, before.DisplayName, stored.DisplayName)
			})
		}
	})
}

func TestUserDelete(t *testing.T) {
	actor := userSignupUser(t, "user_delete_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	victim := userSignupUser(t, "user_delete_victim", "example-DelVic-local-01")
	victim.SessionID = userLoginUser(t, &victim, victim.Password)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		cli := userNewClient(t, actor.SessionID)
		_, err := cli.Delete(victim.UserID)
		userRequireForbidden(t, err)
	})

	t.Run("promote_actor_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)
	})

	t.Run("delete_user_not_found", func(t *testing.T) {
		cli := userNewClient(t, actor.SessionID)
		_, err := cli.Delete("missing-user-id")
		userRequireNotFound(t, err)
	})

	t.Run("delete_superuser_forbidden", func(t *testing.T) {
		cli := userNewClient(t, actor.SessionID)
		superVictim := userSignupUser(t, "user_delete_super_target", "example-UserDelete-local-02")
		userSetSuperuser(t, superVictim.Username, true)

		_, err := cli.Delete(superVictim.UserID)
		userRequireForbidden(t, err)
	})

	t.Run("delete_user_by_id", func(t *testing.T) {
		cli := userNewClient(t, actor.SessionID)
		_, err := cli.Delete(victim.UserID)
		require.NoError(t, err)
		userRequireDeleted(t, victim.Username)
	})

	t.Run("session_invalid_after_delete", func(t *testing.T) {
		userRequireSessionNotFound(t, victim.SessionID)
		userRequireUserSessionNotContains(t, victim.UserID, victim.SessionID)
		userRequireListUnauthorized(t, victim.SessionID)
	})

	t.Run("demote_actor_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, false)
	})
}

func TestUserDeleteMany(t *testing.T) {
	actor := userSignupUser(t, "user_delete_many_actor", "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)

	victim1 := userSignupUser(t, "user_delete_many_victim1", "example-DelMany-local-01")
	victim1.SessionID = userLoginUser(t, &victim1, victim1.Password)

	victim2 := userSignupUser(t, "user_delete_many_victim2", "example-DelMany-local-02")
	victim2.SessionID = userLoginUser(t, &victim2, victim2.Password)

	t.Run("forbidden_when_not_superuser", func(t *testing.T) {
		cli := userNewClient(t, actor.SessionID)
		_, err := cli.DeleteMany([]string{victim1.UserID, victim2.UserID})
		userRequireForbidden(t, err)
	})

	t.Run("promote_actor_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, true)
	})

	t.Run("delete_many_user_not_found", func(t *testing.T) {
		cli := userNewClient(t, actor.SessionID)
		_, err := cli.DeleteMany([]string{victim1.UserID, "missing-user-id"})
		userRequireNotFound(t, err)
	})

	t.Run("delete_many_superuser_forbidden", func(t *testing.T) {
		cli := userNewClient(t, actor.SessionID)
		superVictim := userSignupUser(t, "user_delete_many_super_target", "example-UserDeleteMany-local-03")
		userSetSuperuser(t, superVictim.Username, true)

		_, err := cli.DeleteMany([]string{superVictim.UserID})
		userRequireForbidden(t, err)
	})

	t.Run("delete_many_mixed_targets_forbidden", func(t *testing.T) {
		cli := userNewClient(t, actor.SessionID)
		regularVictim := userSignupUser(t, "user_delete_many_mix_regular", "example-DelMany-local-03")
		superVictim := userSignupUser(t, "user_delete_many_mix_super", "example-DelMany-local-04")
		userSetSuperuser(t, superVictim.Username, true)

		_, err := cli.DeleteMany([]string{regularVictim.UserID, superVictim.UserID})
		userRequireForbidden(t, err)

		require.NotNil(t, userLoadByUsername(t, regularVictim.Username))
		require.NotNil(t, userLoadByUsername(t, superVictim.Username))
	})

	t.Run("root_username_without_root_user_id_cannot_delete_many_superuser", func(t *testing.T) {
		spoofedRootCli := userNewSpoofedRootClient(t, "user_delete_many_spoofed_root")
		superVictim := userSignupUser(t, "user_delete_many_spoofed_root_target", "example-UserDeleteMany-local-05")
		userSetSuperuser(t, superVictim.Username, true)

		_, err := spoofedRootCli.DeleteMany([]string{superVictim.UserID})
		userRequireForbidden(t, err)
		require.NotEmpty(t, userLoadByID(t, superVictim.UserID).ID)
	})

	t.Run("delete_many_users", func(t *testing.T) {
		cli := userNewClient(t, actor.SessionID)
		resp, err := cli.DeleteMany([]string{victim1.UserID, victim2.UserID})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, testSuccessCode, resp.Code)

		userRequireDeleted(t, victim1.Username)
		userRequireDeleted(t, victim2.Username)
	})

	t.Run("sessions_invalid_after_delete_many", func(t *testing.T) {
		userRequireSessionNotFound(t, victim1.SessionID)
		userRequireUserSessionNotContains(t, victim1.UserID, victim1.SessionID)
		userRequireListUnauthorized(t, victim1.SessionID)

		userRequireSessionNotFound(t, victim2.SessionID)
		userRequireUserSessionNotContains(t, victim2.UserID, victim2.SessionID)
		userRequireListUnauthorized(t, victim2.SessionID)
	})

	t.Run("demote_actor_superuser", func(t *testing.T) {
		userSetSuperuser(t, actor.Username, false)
	})
}

func userSignupUser(t *testing.T, prefix, password string) userTestAccount {
	t.Helper()

	user := userTestAccount{
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
		userCleanupUser(t, user.Username)
	})

	return user
}

func userNewClient(t *testing.T, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(userAPI, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

func userNewSpoofedRootClient(t *testing.T, prefix string) *client.Client {
	t.Helper()

	actor := userSignupUser(t, prefix, "12345678")
	actor.SessionID = userLoginUser(t, &actor, actor.Password)
	originalUsername := actor.Username

	userSetUsername(t, actor.UserID, consts.AUTHZ_USER_ROOT)
	t.Cleanup(func() {
		userSetUsername(t, actor.UserID, originalUsername)
	})

	return userNewClient(t, actor.SessionID)
}

func userCleanupUser(t *testing.T, username string) {
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

func userLoadByID(t *testing.T, userID string) *iam.User {
	t.Helper()

	user := new(iam.User)
	require.NoError(t, database.Database[*iam.User](context.Background()).Get(user, userID))
	require.NotEmpty(t, user.ID)
	return user
}

func userLoadByUsername(t *testing.T, username string) *iam.User {
	t.Helper()

	users := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](context.Background()).WithLimit(1).WithQuery(&iam.User{Username: username}).List(&users))
	require.Len(t, users, 1)
	require.NotNil(t, users[0])
	require.NotEmpty(t, users[0].ID)
	return users[0]
}

func userLoginUser(t *testing.T, user *userTestAccount, password string) string {
	t.Helper()

	return loginSessionIDFromCookie(t, user.Username, password)
}

func userSetSuperuser(t *testing.T, username string, enabled bool) {
	t.Helper()

	users := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](context.Background()).WithLimit(1).WithQuery(&iam.User{Username: username}).List(&users))
	require.Len(t, users, 1)

	users[0].IsSuperuser = &enabled
	require.NoError(t, database.Database[*iam.User](context.Background()).Update(users[0]))
}

func userSetUsername(t *testing.T, userID, username string) {
	t.Helper()

	user := userLoadByID(t, userID)
	user.Username = username
	require.NoError(t, database.Database[*iam.User](context.Background()).WithSelect("username").Update(user))
}

func userRequireDeleted(t *testing.T, username string) {
	t.Helper()

	users := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](context.Background()).WithQuery(&iam.User{Username: username}).List(&users))
	require.Empty(t, users)
}

func userRequireSessionNotFound(t *testing.T, sessionID string) {
	t.Helper()

	sessionKey := modeliamsession.SessionIDKey(sessionID)
	_, err := redis.Cache[modeliamsession.Session]().WithContext(t.Context()).Get(sessionKey)
	require.ErrorIs(t, err, types.ErrEntryNotFound)
}

func userRequireUserSessionNotContains(t *testing.T, userID, sessionID string) {
	t.Helper()

	userSessionIDs, err := redis.ZRange(t.Context(), modeliamsession.SessionUserKey(userID), 0, -1)
	require.NoError(t, err)
	require.NotContains(t, userSessionIDs, sessionID)
}

func userRequireListUnauthorized(t *testing.T, sessionID string) {
	t.Helper()

	cli := userNewClient(t, sessionID)
	items := make([]iam.User, 0)
	total := new(int64)
	_, err := cli.List(&items, total)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

func userRequireForbidden(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
	require.Contains(t, err.Error(), `"code":-1`)
}

func userRequirePatchRejected(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)
}

func userRequireNotFound(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}

func userRequireMissingByUsername(t *testing.T, username string) {
	t.Helper()

	users := make([]*iam.User, 0)
	require.NoError(t, database.Database[*iam.User](context.Background()).WithQuery(&iam.User{Username: username}).List(&users))
	require.Empty(t, users)
}

func userFindByUsername(items []*iam.User, username string) *iam.User {
	for _, item := range items {
		if item != nil && item.Username == username {
			return item
		}
	}
	return nil
}
