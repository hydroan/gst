package testutil

import (
	"os"
	"strconv"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestListenOnFreePortConfiguresLocalEphemeralPort(t *testing.T) {
	t.Setenv(config.SERVER_LISTEN, "")
	t.Setenv(config.SERVER_PORT, strconv.Itoa(avoidedPort))

	listenOnFreePort()

	require.NotEqual(t, avoidedPort, serverPort)
	require.Equal(t, "127.0.0.1", os.Getenv(config.SERVER_LISTEN))
	require.Equal(t, strconv.Itoa(serverPort), os.Getenv(config.SERVER_PORT))
}
