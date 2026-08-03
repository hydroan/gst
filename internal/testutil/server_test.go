package testutil

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestListenOnFreePortConfiguresLocalEphemeralPort(t *testing.T) {
	t.Setenv(config.SERVER_LISTEN, "")
	t.Setenv(config.SERVER_PORT, strconv.Itoa(fixedModuleTestPort))

	listenOnFreePort()

	require.NotEqual(t, fixedModuleTestPort, serverPort)
	require.Equal(t, "127.0.0.1", os.Getenv(config.SERVER_LISTEN))
	require.Equal(t, strconv.Itoa(serverPort), os.Getenv(config.SERVER_PORT))
}

func TestURLTargetsTheTestServerPort(t *testing.T) {
	require.Equal(t, fmt.Sprintf("http://127.0.0.1:%d/api/samples", serverPort), URL("/api/samples"))
}
