package testutil

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestSetupRedis(t *testing.T) {
	isolateEnv(t, config.REDIS_ADDR, config.REDIS_ENABLED)

	cleanup, err := setupRedis()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })

	addr := os.Getenv(config.REDIS_ADDR)
	require.NotEmpty(t, addr)
	require.Equal(t, "true", os.Getenv(config.REDIS_ENABLED))

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}
