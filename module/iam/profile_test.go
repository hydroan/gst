package iam_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/database"
	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/module/iam"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

var profileAPI = testutil.URL(port, "/api/iam/profile")

type profileTestAccount struct {
	UserID    string
	Username  string
	Password  string
	SessionID string
}

func TestProfileGet(t *testing.T) {
	account := newProfileTestAccount(t)
	cli := newProfileAuthenticatedClient(t, profileAPI, account.SessionID)

	resp, err := cli.Request(http.MethodGet, &iam.ProfileGetReq{})
	require.NoError(t, err)

	testutil.TestResp(t, resp, func(t *testing.T, rsp iam.ProfileGetRsp) {
		t.Helper()
		require.Equal(t, account.UserID, rsp.UserID)
		require.Empty(t, rsp.ID)
		require.Empty(t, rsp.DisplayName)
		require.Empty(t, rsp.FirstName)
		require.Empty(t, rsp.LastName)
		require.Empty(t, rsp.Avatar)
		require.Empty(t, rsp.Metadata)
	})
	require.Zero(t, profileCountForUser(t, account.UserID))
}

func TestProfilePatch(t *testing.T) {
	account := newProfileTestAccount(t)
	cli := newProfileAuthenticatedClient(t, profileAPI, account.SessionID)

	t.Run("create_profile", func(t *testing.T) {
		displayName := "Profile Test"
		firstName := "Profile"
		avatar := "https://example.com/avatar.png"
		metadata := datatypes.JSONMap{
			"locale": "en-US",
			"public": true,
		}

		resp, err := cli.Request(http.MethodPatch, &iam.ProfilePatchReq{
			DisplayName: &displayName,
			FirstName:   &firstName,
			Avatar:      &avatar,
			Metadata:    metadata,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.ProfilePatchRsp) {
			t.Helper()
			require.NotEmpty(t, rsp.ID)
			require.Equal(t, account.UserID, rsp.UserID)
			require.Equal(t, displayName, rsp.DisplayName)
			require.Equal(t, firstName, rsp.FirstName)
			require.Empty(t, rsp.LastName)
			require.Equal(t, avatar, rsp.Avatar)
			require.Equal(t, metadata, rsp.Metadata)
		})
		require.Equal(t, int64(1), profileCountForUser(t, account.UserID))
	})

	t.Run("patch_only_requested_fields", func(t *testing.T) {
		lastName := "Tester"

		resp, err := cli.Request(http.MethodPatch, &iam.ProfilePatchReq{
			LastName: &lastName,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.ProfilePatchRsp) {
			t.Helper()
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
	})

	t.Run("replace_metadata", func(t *testing.T) {
		metadata := datatypes.JSONMap{
			"timezone": "UTC",
		}

		resp, err := cli.Request(http.MethodPatch, &iam.ProfilePatchReq{
			Metadata: metadata,
		})
		require.NoError(t, err)

		testutil.TestResp(t, resp, func(t *testing.T, rsp iam.ProfilePatchRsp) {
			t.Helper()
			require.Equal(t, account.UserID, rsp.UserID)
			require.Equal(t, "Profile Test", rsp.DisplayName)
			require.Equal(t, "Profile", rsp.FirstName)
			require.Equal(t, "Tester", rsp.LastName)
			require.Equal(t, "https://example.com/avatar.png", rsp.Avatar)
			require.Equal(t, metadata, rsp.Metadata)
		})
	})
}

func newProfileTestAccount(t *testing.T) profileTestAccount {
	t.Helper()

	account := profileTestAccount{
		Username: fmt.Sprintf("profile_%d", time.Now().UnixNano()),
		Password: "12345678",
	}

	cli, err := client.New(signupAPI)
	require.NoError(t, err)

	resp, err := cli.Create(iam.SignupReq{
		Username:   account.Username,
		Password:   account.Password,
		RePassword: account.Password,
	})
	require.NoError(t, err)

	testutil.TestResp(t, resp, func(t *testing.T, rsp iam.SignupRsp) {
		t.Helper()
		require.Equal(t, account.Username, rsp.Username)
		require.NotEmpty(t, rsp.UserID)
		require.NotEmpty(t, rsp.Message)
		account.UserID = rsp.UserID
	})

	account.SessionID = profileLoginSession(t, account.Username, account.Password)
	t.Cleanup(func() {
		profileCleanupAccount(t, account.UserID)
	})

	return account
}

func profileLoginSession(t *testing.T, username, password string) string {
	t.Helper()

	cli, err := client.New(loginAPI)
	require.NoError(t, err)

	resp, err := cli.Create(iam.LoginReq{
		Username: username,
		Password: password,
	})
	require.NoError(t, err)

	testutil.TestResp(t, resp, func(t *testing.T, rsp iam.LoginRsp) {
		t.Helper()
		require.NotEmpty(t, rsp.Principal.UserID)
		require.Equal(t, username, rsp.Principal.Username)
		require.False(t, rsp.ServerTime.IsZero())
	})

	for _, cookie := range resp.Cookies {
		if cookie.Name != "session_id" {
			continue
		}
		require.NotEmpty(t, cookie.Value)
		return cookie.Value
	}

	require.FailNow(t, "session cookie not found")
	return ""
}

func newProfileAuthenticatedClient(t *testing.T, api, sessionID string) *client.Client {
	t.Helper()

	cli, err := client.New(api, client.WithCookie(&http.Cookie{
		Name:  "session_id",
		Value: sessionID,
	}))
	require.NoError(t, err)
	return cli
}

func profileCleanupAccount(t *testing.T, userID string) {
	t.Helper()

	serviceiamsession.InvalidateUserSessions(t.Context(), userID)

	profiles := make([]*modeliamprofile.Profile, 0)
	require.NoError(t, database.Database[*modeliamprofile.Profile](context.Background()).
		WithQuery(&modeliamprofile.Profile{UserID: userID}).
		List(&profiles))
	if len(profiles) > 0 {
		require.NoError(t, database.Database[*modeliamprofile.Profile](context.Background()).Delete(profiles...))
	}

	credentials := make([]*modeliamaccount.PasswordCredential, 0)
	require.NoError(t, database.Database[*modeliamaccount.PasswordCredential](context.Background()).
		WithQuery(&modeliamaccount.PasswordCredential{UserID: userID}).
		List(&credentials))
	if len(credentials) > 0 {
		require.NoError(t, database.Database[*modeliamaccount.PasswordCredential](context.Background()).Delete(credentials...))
	}

	identities := make([]*modeliamaccount.EmailIdentity, 0)
	require.NoError(t, database.Database[*modeliamaccount.EmailIdentity](context.Background()).
		WithQuery(&modeliamaccount.EmailIdentity{UserID: userID}).
		List(&identities))
	if len(identities) > 0 {
		require.NoError(t, database.Database[*modeliamaccount.EmailIdentity](context.Background()).Delete(identities...))
	}

	user := new(iam.User)
	require.NoError(t, database.Database[*iam.User](context.Background()).Get(user, userID))
	require.NoError(t, database.Database[*iam.User](context.Background()).Delete(user))
}

func profileCountForUser(t *testing.T, userID string) int64 {
	t.Helper()

	var total int64
	require.NoError(t, database.Database[*modeliamprofile.Profile](context.Background()).
		WithQuery(&modeliamprofile.Profile{UserID: userID}).
		Count(&total))
	return total
}
