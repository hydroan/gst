package servicemfa

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

type stubAccountAdministrator struct {
	ensure func(*types.ServiceContext, string) error
}

func (s stubAccountAdministrator) EnsureCanAdminister(ctx *types.ServiceContext, targetUserID string) error {
	return s.ensure(ctx, targetUserID)
}

func TestMissingAccountAdministratorDeniesEverything(t *testing.T) {
	SetAccountAdministrator(nil)

	err := currentAccountAdministrator().EnsureCanAdminister(&types.ServiceContext{}, "user-1")

	require.ErrorIs(t, err, ErrAccountAdministratorNotConfigured)
	var svcErr *service.Error
	require.True(t, errors.As(err, &svcErr))
	require.Equal(t, http.StatusInternalServerError, svcErr.Status())
}

func TestSetAccountAdministratorInstallsAndResets(t *testing.T) {
	t.Cleanup(func() { SetAccountAdministrator(nil) })

	var gotTarget string
	SetAccountAdministrator(stubAccountAdministrator{ensure: func(_ *types.ServiceContext, targetUserID string) error {
		gotTarget = targetUserID
		return nil
	}})
	require.NoError(t, currentAccountAdministrator().EnsureCanAdminister(&types.ServiceContext{}, "user-2"))
	require.Equal(t, "user-2", gotTarget)

	SetAccountAdministrator(nil)
	require.ErrorIs(t,
		currentAccountAdministrator().EnsureCanAdminister(&types.ServiceContext{}, "user-2"),
		ErrAccountAdministratorNotConfigured)
}
