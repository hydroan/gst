package testcontainer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestSetupSqlite(t *testing.T) {
	isolateEnv(t,
		config.SQLITE_PATH, config.SQLITE_IS_MEMORY,
		config.DATABASE_TYPE, config.DATABASE_AUTO_MIGRATE)

	cleanup, err := setupSqlite()
	require.NoError(t, err)

	path := os.Getenv(config.SQLITE_PATH)
	require.NotEmpty(t, path)
	require.DirExists(t, filepath.Dir(path))
	// An in-memory database is shared by the whole process, which would defeat
	// the isolation this setup provides: the setup names the file mode
	// outright.
	require.Equal(t, "false", os.Getenv(config.SQLITE_IS_MEMORY))
	require.Equal(t, string(config.DBSqlite), os.Getenv(config.DATABASE_TYPE))
	require.Equal(t, "true", os.Getenv(config.DATABASE_AUTO_MIGRATE))

	require.NoError(t, cleanup())
	require.NoDirExists(t, filepath.Dir(path))
}
