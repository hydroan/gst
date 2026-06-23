package email

import (
	"testing"
	"time"

	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestIAMUserSnapshotMapsMinimalEmailUserState(t *testing.T) {
	email := "User@Example.COM"
	verified := true
	user := &modeliamuser.User{
		Base:          model.Base{ID: "user-1"},
		Status:        modeliamuser.UserStatusActive,
		Email:         &email,
		EmailVerified: &verified,
	}

	snapshot := iamUserSnapshot(user)

	require.Equal(t, "user-1", snapshot.ID)
	require.Equal(t, "User@Example.COM", snapshot.Email)
	require.True(t, snapshot.Active)
	require.True(t, snapshot.EmailVerified)
}

func TestIAMUserSnapshotMarksInactiveAccountsInactive(t *testing.T) {
	user := &modeliamuser.User{
		Base:   model.Base{ID: "user-2"},
		Status: modeliamuser.UserStatusLocked,
	}

	snapshot := iamUserSnapshot(user)

	require.Equal(t, "user-2", snapshot.ID)
	require.False(t, snapshot.Active)
}

func TestApplyIAMPasswordResetHashesPasswordAndClearsChangeFlag(t *testing.T) {
	user := &modeliamuser.User{MustChangePassword: true}

	err := applyIAMPasswordReset(user, "new-password-123")

	require.NoError(t, err)
	require.False(t, user.MustChangePassword)
	require.NotEqual(t, "new-password-123", user.PasswordHash)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("new-password-123")))
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
