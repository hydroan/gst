package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadAppliesFrameworkDefaults(t *testing.T) {
	// An empty file must carry exactly the runtime default configuration.
	c, err := Load(writeLoadFile(t, "config.ini", ""))
	require.NoError(t, err)

	require.Equal(t, DBSqlite, c.Database.Type)
	require.True(t, c.Sqlite.Enabled)
	// -1 keeps the NATS client reconnecting forever; see config.Nats.MaxReconnects.
	require.Equal(t, -1, c.Nats.MaxReconnects)
	// Dial is bounded by default, per-connection I/O deadlines are not; see
	// the field comments on config.MySQL.
	require.Equal(t, 10*time.Second, c.MySQL.DialTimeout)
	require.Zero(t, c.MySQL.ReadTimeout)
	require.Zero(t, c.MySQL.WriteTimeout)
}

func TestLoadReadsFileContent(t *testing.T) {
	c, err := Load(writeLoadFile(t, "config.ini", "[database]\ntype = mysql\n\n[mysql]\nenabled = false\n"))
	require.NoError(t, err)

	require.Equal(t, DBMySQL, c.Database.Type)
	require.False(t, c.MySQL.Enabled)
}

func TestLoadIgnoresEnvironment(t *testing.T) {
	// Load is deterministic across machines: environment overrides that the
	// runtime honors are deliberately not applied.
	t.Setenv("DATABASE_TYPE", "postgres")

	c, err := Load(writeLoadFile(t, "config.ini", "[database]\ntype = mysql\n"))
	require.NoError(t, err)

	require.Equal(t, DBMySQL, c.Database.Type)
}

func TestLoadReadFailure(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "absent.ini"))
	require.Error(t, err)
}

// writeLoadFile writes content into a fresh temp directory and returns the
// file path.
func writeLoadFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
