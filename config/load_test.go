package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestLoadAppliesFrameworkDefaults(t *testing.T) {
	// An empty file must carry exactly the runtime default configuration.
	c, err := config.Load(writeLoadFile(t, "config.ini", ""))
	require.NoError(t, err)

	// A deployment names its database: no dialect is selected by default,
	// and the SQLite one, once selected, is the file, not a database held in
	// memory.
	require.Empty(t, c.Database.Type)
	require.False(t, c.Sqlite.IsMemory)
	// -1 keeps the NATS client reconnecting forever; see config.Nats.MaxReconnects.
	require.Equal(t, -1, c.Nats.MaxReconnects)
	// Dial is bounded by default, per-connection I/O deadlines are not; see
	// the field comments on config.MySQL.
	require.Equal(t, 10*time.Second, c.MySQL.DialTimeout)
	require.Zero(t, c.MySQL.ReadTimeout)
	require.Zero(t, c.MySQL.WriteTimeout)
	// Connections add the strict mode by default; see config.MySQL.Strict.
	require.True(t, c.MySQL.Strict)
	// A redis client presents no certificate by default: neither file of
	// the key pair is named.
	require.Empty(t, c.Redis.CertFile)
	require.Empty(t, c.Redis.KeyFile)
}

func TestLoadReadsFileContent(t *testing.T) {
	c, err := config.Load(writeLoadFile(t, "config.ini", "[database]\ntype = mysql\n\n[mysql]\nhost = db.internal\n"))
	require.NoError(t, err)

	require.Equal(t, config.DBMySQL, c.Database.Type)
	require.Equal(t, "db.internal", c.MySQL.Host)
}

func TestLoadIgnoresEnvironment(t *testing.T) {
	// Load is deterministic across machines: environment overrides that the
	// runtime honors are deliberately not applied.
	t.Setenv("DATABASE_TYPE", "postgres")

	c, err := config.Load(writeLoadFile(t, "config.ini", "[database]\ntype = mysql\n"))
	require.NoError(t, err)

	require.Equal(t, config.DBMySQL, c.Database.Type)
}

func TestLoadReadFailure(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "absent.ini"))
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

// TestLoadRefusesAKeyTheFrameworkSectionLacks pins that Load reads a file
// the way Init does: a key one of the framework's own sections does not have
// fails it naming the section and the key, and a section the framework does
// not know passes.
func TestLoadRefusesAKeyTheFrameworkSectionLacks(t *testing.T) {
	_, err := config.Load(writeLoadFile(t, "config.ini", "[logger]\nleve = debug\n"))
	require.ErrorContains(t, err, `section "logger" has no key "leve"`)

	c, err := config.Load(writeLoadFile(t, "config.ini", "[nosuchsection]\nkey = 1\n\n[logger]\nlevel = debug\n"))
	require.NoError(t, err)
	require.Equal(t, config.LogLevelDebug, c.Logger.Level)
}
