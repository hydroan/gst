package serviceiamsession_test

import (
	"testing"
	"time"

	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/stretchr/testify/require"
)

func TestNewSessionIDGeneratesOpaqueRandomToken(t *testing.T) {
	first, err := serviceiamsession.NewSessionID()
	require.NoError(t, err)
	require.Regexp(t, `^[0-9a-f]{64}$`, first)
	require.NotRegexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, first)

	second, err := serviceiamsession.NewSessionID()
	require.NoError(t, err)
	require.Regexp(t, `^[0-9a-f]{64}$`, second)
	require.NotEqual(t, first, second)
}

func TestGetSessionUserStateTTL(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("IAM_SESSION_USER_STATE_TTL", "")

		require.Equal(t, 30*time.Second, serviceiamsession.GetSessionUserStateTTL())
	})

	t.Run("environment_override", func(t *testing.T) {
		t.Setenv("IAM_SESSION_USER_STATE_TTL", "45s")

		require.Equal(t, 45*time.Second, serviceiamsession.GetSessionUserStateTTL())
	})
}
