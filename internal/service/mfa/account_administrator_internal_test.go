package servicemfa

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"github.com/stretchr/testify/require"
)

type stubAccountAdministrator struct {
	ensure func(*gst.ServiceContext, string) error
}

func (s stubAccountAdministrator) EnsureCanAdminister(ctx *gst.ServiceContext, targetUserID string) error {
	return s.ensure(ctx, targetUserID)
}

func TestMissingAccountAdministratorDeniesEverything(t *testing.T) {
	SetAccountAdministrator(nil)

	err := currentAccountAdministrator().EnsureCanAdminister(&gst.ServiceContext{}, "user-1")

	require.ErrorIs(t, err, ErrAccountAdministratorNotConfigured)
	var svcErr *service.Error
	require.True(t, errors.As(err, &svcErr))
	require.Equal(t, http.StatusInternalServerError, svcErr.Status())
}

func TestSetAccountAdministratorInstallsAndResets(t *testing.T) {
	t.Cleanup(func() { SetAccountAdministrator(nil) })

	var gotTarget string
	SetAccountAdministrator(stubAccountAdministrator{ensure: func(_ *gst.ServiceContext, targetUserID string) error {
		gotTarget = targetUserID
		return nil
	}})
	require.NoError(t, currentAccountAdministrator().EnsureCanAdminister(&gst.ServiceContext{}, "user-2"))
	require.Equal(t, "user-2", gotTarget)

	SetAccountAdministrator(nil)
	require.ErrorIs(t,
		currentAccountAdministrator().EnsureCanAdminister(&gst.ServiceContext{}, "user-2"),
		ErrAccountAdministratorNotConfigured)
}
