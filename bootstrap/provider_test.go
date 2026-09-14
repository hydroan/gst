package bootstrap

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/stretchr/testify/require"

	// Every provider package registers itself from init. Importing them all
	// here lets the tests below prove that each package under provider/
	// self-registers under its package name and gets its own log file; a new
	// provider package fails them until it self-registers and joins this
	// import list.
	_ "github.com/hydroan/gst/provider/cassandra"
	_ "github.com/hydroan/gst/provider/clickhouse"
	_ "github.com/hydroan/gst/provider/elastic"
	_ "github.com/hydroan/gst/provider/etcd"
	_ "github.com/hydroan/gst/provider/influxdb"
	_ "github.com/hydroan/gst/provider/kafka"
	_ "github.com/hydroan/gst/provider/ldap"
	_ "github.com/hydroan/gst/provider/minio"
	_ "github.com/hydroan/gst/provider/mongo"
	_ "github.com/hydroan/gst/provider/mqtt"
	_ "github.com/hydroan/gst/provider/nats"
	_ "github.com/hydroan/gst/provider/rethinkdb"
	_ "github.com/hydroan/gst/provider/rocketmq"
	_ "github.com/hydroan/gst/provider/scylla"
)

// TestEveryProviderPackageSelfRegisters walks the provider/ directory and
// requires a registration for each package: a provider that forgot its init
// registration would never start, and its first use would fail in production
// instead of here.
func TestEveryProviderPackageSelfRegisters(t *testing.T) {
	registered := make(map[string]bool)
	for _, c := range lifecycle.Components(lifecycle.StageProvider) {
		registered[c.Name] = true
	}

	for name := range providerDirectories(t) {
		require.True(t, registered[name], "provider package %q must self-register under its package name", name)
	}
}

// TestEveryProviderGetsItsOwnLogFile proves the promise a provider package
// gets for declaring SetLogger: once the process is bootstrapped, a logger
// writing <name>.log exists in the configured log directory for every
// compiled-in provider, enabled or not. Every provider declares it, except
// clickhouse, which has no logger of its own. The default configuration
// leaves every provider disabled, so the bootstrap binds the loggers without
// connecting to anything.
func TestEveryProviderGetsItsOwnLogFile(t *testing.T) {
	bootstrapProcess(t)

	for _, c := range lifecycle.Components(lifecycle.StageProvider) {
		if c.Name == "clickhouse" {
			require.Nil(t, c.SetLogger, "clickhouse logs through no logger of its own")
			continue
		}
		require.NotNil(t, c.SetLogger, "provider %q must declare SetLogger to get its own log file", c.Name)
		require.FileExists(t, filepath.Join(bootstrapLogDir, c.Name+".log"), "provider %q must get its own log file", c.Name)
	}
}

// providerDirectories returns the names of the packages under provider/.
func providerDirectories(t *testing.T) map[string]bool {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	entries, err := os.ReadDir(filepath.Join(filepath.Dir(file), "..", "provider"))
	require.NoError(t, err)

	dirs := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			dirs[entry.Name()] = true
		}
	}
	return dirs
}
