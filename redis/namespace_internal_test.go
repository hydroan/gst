package redis

import (
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestRedisKeyUsesConfiguredNamespace(t *testing.T) {
	originalNamespace := config.App.Redis.Namespace
	t.Cleanup(func() {
		config.App.Redis.Namespace = originalNamespace
	})

	config.App.Redis.Namespace = "module-test"

	require.Equal(t, "module-test:iam:session:id:123", Key("iam:session:id:123"))
	require.Equal(t, "module-test:iam:session*", pattern("iam:session"))
	require.Equal(t, "module-test:iam:session*", pattern("iam:session*"))
	require.Equal(t, "module-test:*", pattern(""))
	require.Equal(t, "module-test:already", Key("module-test:already"))
}

func TestRedisKeyAllowsEmptyNamespace(t *testing.T) {
	originalNamespace := config.App.Redis.Namespace
	t.Cleanup(func() {
		config.App.Redis.Namespace = originalNamespace
	})

	config.App.Redis.Namespace = ""

	require.Equal(t, "iam:session:id:123", Key("iam:session:id:123"))
	require.Equal(t, "iam:session*", pattern("iam:session"))
	require.Equal(t, "*", pattern(""))
}

// TestKeyMatchesWhatOperationsWrite is the reason Key is exported: a caller
// working through the Client handle — for a data structure this package does
// not wrap — has to land on the same key the package's own operations use.
func TestKeyMatchesWhatOperationsWrite(t *testing.T) {
	originalNamespace := config.App.Redis.Namespace
	t.Cleanup(func() {
		config.App.Redis.Namespace = originalNamespace
	})
	config.App.Redis.Namespace = "namespace-test"

	client, err := Client()
	require.NoError(t, err)
	require.NoError(t, client.Set(t.Context(), Key("through-the-handle"), "value", time.Minute).Err())

	stored, err := Get(t.Context(), "through-the-handle")
	require.NoError(t, err)
	require.Equal(t, "value", string(stored))
}
