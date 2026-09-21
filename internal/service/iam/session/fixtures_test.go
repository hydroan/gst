package serviceiamsession_test

import (
	"context"
	"testing"

	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/redis"
	"github.com/stretchr/testify/require"
)

// clearSessions drops the session keys left by earlier tests in this run. The
// redis container is fresh per run, so this is only about keeping the tests in
// this package from seeing each other's sessions.
func clearSessions(t *testing.T) {
	t.Helper()

	// Both namespaces, because the user-state cache is keyed by user and is
	// therefore deliberately outside the session prefix.
	require.NoError(t, redis.RemovePrefix(context.Background(), serviceiamsession.SessionNamespace))
	require.NoError(t, redis.RemovePrefix(context.Background(), serviceiamsession.UserNamespace))
}
