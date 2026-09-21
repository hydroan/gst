package iam

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestRequireRedisEnabled covers the startup guard that refuses an IAM
// registered without the Redis every session lives in.
//
// The check has to fail at startup rather than at the first login: a
// deployment can still fix its configuration while it is starting, and a 500
// per login attempt names neither the cause nor the fix.
func TestRequireRedisEnabled(t *testing.T) {
	enabled := config.App.Redis.Enabled
	t.Cleanup(func() { config.App.Redis.Enabled = enabled })

	t.Run("passes_when_redis_is_enabled", func(t *testing.T) {
		config.App.Redis.Enabled = true

		require.NoError(t, requireRedisEnabled())
	})

	t.Run("fails_when_redis_is_disabled", func(t *testing.T) {
		config.App.Redis.Enabled = false

		err := requireRedisEnabled()
		require.Error(t, err)
		require.Contains(t, err.Error(), config.REDIS_ENABLED)
	})
}
