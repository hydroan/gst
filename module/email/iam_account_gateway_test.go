package email

import (
	"testing"
	"time"

	modeliamaccount "github.com/hydroan/gst/internal/model/iam/account"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestIAMAccountSnapshotMapsMinimalEmailAccountState(t *testing.T) {
	email := "User@Example.COM"
	verified := true
	user := &modeliamuser.User{
		Base:          model.Base{ID: "user-1"},
		Status:        modeliamuser.UserStatusActive,
		Email:         &email,
		EmailVerified: &verified,
	}

	snapshot := iamAccountSnapshot(user)

	require.Equal(t, "user-1", snapshot.ID)
	require.Equal(t, "User@Example.COM", snapshot.Email)
	require.True(t, snapshot.Active)
	require.True(t, snapshot.EmailVerified)
}

func TestIAMAccountSnapshotMarksInactiveAccountsInactive(t *testing.T) {
	user := &modeliamuser.User{
		Base:   model.Base{ID: "user-2"},
		Status: modeliamuser.UserStatusLocked,
	}

	snapshot := iamAccountSnapshot(user)

	require.Equal(t, "user-2", snapshot.ID)
	require.False(t, snapshot.Active)
}

func TestApplyIAMPasswordUpdateHashesPasswordAndClearsChangeFlag(t *testing.T) {
	credential := &modeliamaccount.PasswordCredential{MustChangePassword: true}

	err := applyIAMPasswordUpdate(credential, "new-password-123")

	require.NoError(t, err)
	require.False(t, credential.MustChangePassword)
	require.NotEqual(t, "new-password-123", credential.PasswordHash)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte("new-password-123")))
}

func TestApplyIAMEmailChangeNormalizesAndMarksVerified(t *testing.T) {
	user := new(modeliamuser.User)
	changedAt := time.Date(2026, 3, 31, 15, 30, 0, 0, time.FixedZone("CST", 8*60*60))

	err := applyIAMEmailChange(user, " New@Example.COM ", changedAt)

	require.NoError(t, err)
	require.NotNil(t, user.Email)
	require.Equal(t, "new@example.com", *user.Email)
	require.NotNil(t, user.EmailVerified)
	require.True(t, *user.EmailVerified)
	require.NotNil(t, user.EmailVerifiedAt)
	require.Equal(t, changedAt.UTC(), *user.EmailVerifiedAt)
	require.NotNil(t, user.LastEmailChangedAt)
	require.Equal(t, changedAt.UTC(), *user.LastEmailChangedAt)
}
