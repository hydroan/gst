package redis

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestRedisKeyUsesConfiguredNamespace(t *testing.T) {
	originalNamespace := config.App.Redis.Namespace
	t.Cleanup(func() {
		config.App.Redis.Namespace = originalNamespace
	})

	config.App.Redis.Namespace = "module-test"

	require.Equal(t, "module-test:iam:session:id:123", redisKey("iam:session:id:123"))
	require.Equal(t, "module-test:iam:session*", redisPattern("iam:session"))
	require.Equal(t, "module-test:iam:session*", redisPattern("iam:session*"))
	require.Equal(t, "module-test:*", redisPattern(""))
	require.Equal(t, "module-test:already", redisKey("module-test:already"))
}

func TestRedisKeyAllowsEmptyNamespace(t *testing.T) {
	originalNamespace := config.App.Redis.Namespace
	t.Cleanup(func() {
		config.App.Redis.Namespace = originalNamespace
	})

	config.App.Redis.Namespace = ""

	require.Equal(t, "iam:session:id:123", redisKey("iam:session:id:123"))
	require.Equal(t, "iam:session*", redisPattern("iam:session"))
	require.Equal(t, "*", redisPattern(""))
}
