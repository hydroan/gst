package serviceiamsession_test

import (
	"testing"

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
