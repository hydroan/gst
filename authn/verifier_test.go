package authn_test

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/authn"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

func TestVerifyLoginSecondFactorWithoutVerifierPasses(t *testing.T) {
	authn.SetLoginSecondFactorVerifier(nil)

	require.NoError(t, authn.VerifyLoginSecondFactor(&types.ServiceContext{}, "user-1", authn.LoginSecondFactor{
		TOTPCode: "123456",
	}))
}

func TestSetLoginSecondFactorVerifierInstallsGate(t *testing.T) {
	t.Cleanup(func() { authn.SetLoginSecondFactorVerifier(nil) })

	ctx := &types.ServiceContext{}
	gateErr := service.NewError(http.StatusUnauthorized, authn.MsgSecondFactorRequired)

	var gotCtx *types.ServiceContext
	var gotUserID string
	var gotFactor authn.LoginSecondFactor
	authn.SetLoginSecondFactorVerifier(func(ctx *types.ServiceContext, userID string, factor authn.LoginSecondFactor) error {
		gotCtx = ctx
		gotUserID = userID
		gotFactor = factor
		return gateErr
	})

	err := authn.VerifyLoginSecondFactor(ctx, "user-1", authn.LoginSecondFactor{
		TOTPCode:   "123456",
		BackupCode: "backup-1",
	})

	require.Same(t, ctx, gotCtx)
	require.Equal(t, "user-1", gotUserID)
	require.Equal(t, authn.LoginSecondFactor{TOTPCode: "123456", BackupCode: "backup-1"}, gotFactor)
	require.True(t, errors.Is(err, gateErr))
}

func TestSetLoginSecondFactorVerifierRejectsSecondInstall(t *testing.T) {
	t.Cleanup(func() { authn.SetLoginSecondFactorVerifier(nil) })

	authn.SetLoginSecondFactorVerifier(func(*types.ServiceContext, string, authn.LoginSecondFactor) error {
		return nil
	})

	require.Panics(t, func() {
		authn.SetLoginSecondFactorVerifier(func(*types.ServiceContext, string, authn.LoginSecondFactor) error {
			return nil
		})
	})
}

func TestSetLoginSecondFactorVerifierNilUninstalls(t *testing.T) {
	t.Cleanup(func() { authn.SetLoginSecondFactorVerifier(nil) })

	sentinel := errors.New("gate closed")
	authn.SetLoginSecondFactorVerifier(func(*types.ServiceContext, string, authn.LoginSecondFactor) error {
		return sentinel
	})
	require.ErrorIs(t, authn.VerifyLoginSecondFactor(&types.ServiceContext{}, "user-1", authn.LoginSecondFactor{}), sentinel)

	authn.SetLoginSecondFactorVerifier(nil)
	require.NoError(t, authn.VerifyLoginSecondFactor(&types.ServiceContext{}, "user-1", authn.LoginSecondFactor{}))

	// Uninstalling frees the slot for a later install.
	authn.SetLoginSecondFactorVerifier(func(*types.ServiceContext, string, authn.LoginSecondFactor) error {
		return sentinel
	})
	require.ErrorIs(t, authn.VerifyLoginSecondFactor(&types.ServiceContext{}, "user-1", authn.LoginSecondFactor{}), sentinel)
}
