package testutil

import (
	"os"
	"strconv"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestListenOnFreePortConfiguresLocalEphemeralPorts(t *testing.T) {
	for _, key := range []string{config.SERVER_LISTEN, config.SERVER_PORT, config.GRPC_LISTEN, config.GRPC_PORT} {
		t.Setenv(key, "")
	}

	listenOnFreePort()

	require.Positive(t, serverPort)
	require.Positive(t, grpcPort)
	require.NotEqual(t, serverPort, grpcPort)
	require.Equal(t, loopbackHost, os.Getenv(config.SERVER_LISTEN))
	require.Equal(t, strconv.Itoa(serverPort), os.Getenv(config.SERVER_PORT))
	require.Equal(t, loopbackHost, os.Getenv(config.GRPC_LISTEN))
	require.Equal(t, strconv.Itoa(grpcPort), os.Getenv(config.GRPC_PORT))
}
