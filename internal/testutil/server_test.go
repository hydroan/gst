package testutil

import (
	"os"
	"strconv"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestSetupRandomServerPortConfiguresLocalEphemeralPort(t *testing.T) {
	t.Setenv(config.SERVER_LISTEN, "")
	t.Setenv(config.SERVER_PORT, "8000")

	port := SetupRandomServerPort()

	require.NotEqual(t, 8000, port)
	require.Equal(t, "127.0.0.1", os.Getenv(config.SERVER_LISTEN))
	require.Equal(t, strconv.Itoa(port), os.Getenv(config.SERVER_PORT))
}

func TestSetupRandomRedisNamespaceConfiguresUniqueNamespace(t *testing.T) {
	t.Setenv(config.REDIS_NAMESPACE, "gst")

	namespace1 := SetupRandomRedisNamespace()
	namespace2 := SetupRandomRedisNamespace()

	require.NotEmpty(t, namespace1)
	require.NotEmpty(t, namespace2)
	require.NotEqual(t, namespace1, namespace2)
	require.Equal(t, namespace2, os.Getenv(config.REDIS_NAMESPACE))
}
