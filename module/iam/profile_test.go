package iam_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
	"github.com/hydroan/gst/module/iam"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

const profilePath = "/api/iam/profile"

type profileTestAccount struct {
	UserID    string
	Username  string
	Password  string
	SessionID string
}

func TestProfileGet(t *testing.T) {
	account := newProfileTestAccount(t)
	cli := sessionClient(t, account.SessionID)

	rsp, err := cli.Get[iam.ProfileGetRsp](profilePath)
	require.NoError(t, err)

	require.Equal(t, account.UserID, rsp.UserID)
	require.Empty(t, rsp.ID)
	require.Empty(t, rsp.DisplayName)
	require.Empty(t, rsp.FirstName)
	require.Empty(t, rsp.LastName)
	require.Empty(t, rsp.Avatar)
	require.Empty(t, rsp.Metadata)
	require.Zero(t, profileCountForUser(t, account.UserID))
}

func TestProfilePatch(t *testing.T) {
	account := newProfileTestAccount(t)
	cli := sessionClient(t, account.SessionID)

	t.Run("create_profile", func(t *testing.T) {
		displayName := "Profile Test"
		firstName := "Profile"
		avatar := "https://example.com/avatar.png"
		metadata := datatypes.JSONMap{
			"locale": "en-US",
			"public": true,
		}

		rsp, err := cli.Patch[iam.ProfilePatchRsp](profilePath, &iam.ProfilePatchReq{
			DisplayName: &displayName,
			FirstName:   &firstName,
			Avatar:      &avatar,
			Metadata:    metadata,
		})
		require.NoError(t, err)

		require.NotEmpty(t, rsp.ID)
		require.Equal(t, account.UserID, rsp.UserID)
		require.Equal(t, displayName, rsp.DisplayName)
		require.Equal(t, firstName, rsp.FirstName)
		require.Empty(t, rsp.LastName)
		require.Equal(t, avatar, rsp.Avatar)
		require.Equal(t, metadata, rsp.Metadata)
		require.Equal(t, 1, profileCountForUser(t, account.UserID))
	})

	t.Run("patch_only_requested_fields", func(t *testing.T) {
		lastName := "Tester"

		rsp, err := cli.Patch[iam.ProfilePatchRsp](profilePath, &iam.ProfilePatchReq{
			LastName: &lastName,
		})
		require.NoError(t, err)

		require.Equal(t, account.UserID, rsp.UserID)
		require.Equal(t, "Profile Test", rsp.DisplayName)
		require.Equal(t, "Profile", rsp.FirstName)
		require.Equal(t, lastName, rsp.LastName)
		require.Equal(t, "https://example.com/avatar.png", rsp.Avatar)
		require.Equal(t, datatypes.JSONMap{
			"locale": "en-US",
			"public": true,
		}, rsp.Metadata)
	})

	t.Run("replace_metadata", func(t *testing.T) {
		metadata := datatypes.JSONMap{
			"timezone": "UTC",
		}

		rsp, err := cli.Patch[iam.ProfilePatchRsp](profilePath, &iam.ProfilePatchReq{
			Metadata: metadata,
		})
		require.NoError(t, err)

		require.Equal(t, account.UserID, rsp.UserID)
		require.Equal(t, "Profile Test", rsp.DisplayName)
		require.Equal(t, "Profile", rsp.FirstName)
		require.Equal(t, "Tester", rsp.LastName)
		require.Equal(t, "https://example.com/avatar.png", rsp.Avatar)
		require.Equal(t, metadata, rsp.Metadata)
	})
}

func newProfileTestAccount(t *testing.T) profileTestAccount {
	t.Helper()

	account := profileTestAccount{
		Username: fmt.Sprintf("profile_%d", time.Now().UnixNano()),
		Password: "12345678",
	}

	cli, err := client.New(baseURL)
	require.NoError(t, err)

	rsp, err := cli.Post[iam.SignupRsp](signupPath, iam.SignupReq{
		Username:   account.Username,
		Password:   account.Password,
		RePassword: account.Password,
	})
	require.NoError(t, err)
	require.Equal(t, account.Username, rsp.Username)
	require.NotEmpty(t, rsp.UserID)
	require.NotEmpty(t, rsp.Message)
	account.UserID = rsp.UserID

	account.SessionID = profileLoginSession(t, account.Username, account.Password)

	return account
}

func profileLoginSession(t *testing.T, username, password string) string {
	t.Helper()

	cli, err := client.New(baseURL)
	require.NoError(t, err)

	resp, err := cli.Do(http.MethodPost, loginPath, iam.LoginReq{
		Username: username,
		Password: password,
	})
	require.NoError(t, err)

	cookie := resp.Cookie("session_id")
	require.NotNil(t, cookie, "session cookie not found")
	require.NotEmpty(t, cookie.Value)
	return cookie.Value
}

func profileCountForUser(t *testing.T, userID string) int {
	t.Helper()

	var total int
	require.NoError(t, database.Database[*modeliamprofile.Profile](context.Background()).
		WithQuery(&modeliamprofile.Profile{UserID: userID}).
		Count(&total))
	return total
}
