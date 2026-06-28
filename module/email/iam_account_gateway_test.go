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
	verifiedAt := time.Date(2026, 3, 31, 15, 30, 0, 0, time.UTC)
	user := &modeliamuser.User{
		Base:   model.Base{ID: "user-1"},
		Status: modeliamuser.UserStatusActive,
	}
	identity := &modeliamaccount.EmailIdentity{
		UserID:     user.ID,
		Email:      email,
		VerifiedAt: &verifiedAt,
	}

	snapshot := iamAccountSnapshot(user, identity)

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

	snapshot := iamAccountSnapshot(user, nil)

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
	identity := new(modeliamaccount.EmailIdentity)
	changedAt := time.Date(2026, 3, 31, 15, 30, 0, 0, time.FixedZone("CST", 8*60*60))

	err := applyIAMEmailChange(identity, " New@Example.COM ", changedAt)

	require.NoError(t, err)
	require.Equal(t, "New@Example.COM", identity.Email)
	require.Equal(t, "new@example.com", identity.NormalizedEmail)
	require.NotNil(t, identity.VerifiedAt)
	require.Equal(t, changedAt.UTC(), *identity.VerifiedAt)
	require.NotNil(t, identity.LastChangedAt)
	require.Equal(t, changedAt.UTC(), *identity.LastChangedAt)
}
