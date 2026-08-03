package testcontainer

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestSetupSqlite(t *testing.T) {
	isolateEnv(t,
		config.SQLITE_PATH, config.SQLITE_DATABASE, config.SQLITE_IS_MEMORY,
		config.DATABASE_TYPE, config.DATABASE_AUTO_MIGRATE)

	cleanup, err := setupSqlite()
	require.NoError(t, err)

	path := os.Getenv(config.SQLITE_PATH)
	require.NotEmpty(t, path)
	require.DirExists(t, filepath.Dir(path))
	// The framework defaults sqlite to an in-memory database shared by the
	// whole process, which would defeat the isolation this setup provides.
	require.Equal(t, "false", os.Getenv(config.SQLITE_IS_MEMORY))
	require.Equal(t, string(config.DBSqlite), os.Getenv(config.DATABASE_TYPE))
	require.Equal(t, "true", os.Getenv(config.DATABASE_AUTO_MIGRATE))

	require.NoError(t, cleanup())
	require.NoDirExists(t, filepath.Dir(path))
}

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
	require.Equal(t, "test", os.Getenv(config.POSTGRES_DATABASE))
	require.Equal(t, "test", os.Getenv(config.POSTGRES_USERNAME))
	require.Equal(t, "test", os.Getenv(config.POSTGRES_PASSWORD))
	require.Equal(t, "disable", os.Getenv(config.POSTGRES_SSLMODE))
	require.Equal(t, string(config.DBPostgres), os.Getenv(config.DATABASE_TYPE))
	require.Equal(t, "true", os.Getenv(config.DATABASE_AUTO_MIGRATE))

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}
