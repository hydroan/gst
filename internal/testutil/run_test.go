package testutil

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestServerPreparesClickhouse(t *testing.T) {
	for _, key := range []string{config.CLICKHOUSE_HOST, config.CLICKHOUSE_PORT, config.CLICKHOUSE_ENABLED} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}

	release, err := Server{Clickhouse: true}.prepare()
	t.Cleanup(release)
	require.NoError(t, err)

	// The prepared instance lands in the environment the way the other
	// services do, which is where the bootstrap config picks it up.
	host := os.Getenv(config.CLICKHOUSE_HOST)
	port := os.Getenv(config.CLICKHOUSE_PORT)
	require.NotEmpty(t, host)
	require.NotEmpty(t, port)
	require.Equal(t, "true", os.Getenv(config.CLICKHOUSE_ENABLED))

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}
