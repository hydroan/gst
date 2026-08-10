package testcontainer

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestSetupPostgres(t *testing.T) {
	isolateEnv(t,
		config.POSTGRES_HOST, config.POSTGRES_PORT, config.POSTGRES_DATABASE,
		config.POSTGRES_USERNAME, config.POSTGRES_PASSWORD, config.POSTGRES_SSLMODE,
		config.DATABASE_TYPE, config.DATABASE_AUTO_MIGRATE)

	cleanup, err := setupPostgres()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })

	host := os.Getenv(config.POSTGRES_HOST)
	port := os.Getenv(config.POSTGRES_PORT)
	require.NotEmpty(t, host)
	require.NotEmpty(t, port)
	// The default path shares the container, so the binary runs as the
	// superuser on a provisioned database of its own.
	require.Regexp(t, sharedDatabasePattern, os.Getenv(config.POSTGRES_DATABASE))
	require.Equal(t, postgresSuperUsername, os.Getenv(config.POSTGRES_USERNAME))
	require.Equal(t, "test", os.Getenv(config.POSTGRES_PASSWORD))
	require.Equal(t, "disable", os.Getenv(config.POSTGRES_SSLMODE))
	require.Equal(t, string(config.DBPostgres), os.Getenv(config.DATABASE_TYPE))
	require.Equal(t, "true", os.Getenv(config.DATABASE_AUTO_MIGRATE))

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}
