package provider_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hydroan/gst/provider"
	"github.com/stretchr/testify/require"

	// Every provider package registers itself from init. Importing them all
	// here lets the test below prove that each package under provider/
	// self-registers under its package name; a new provider package fails
	// the test until it both self-registers and joins this import list.
	_ "github.com/hydroan/gst/provider/cassandra"
	_ "github.com/hydroan/gst/provider/elastic"
	_ "github.com/hydroan/gst/provider/etcd"
	_ "github.com/hydroan/gst/provider/influxdb"
	_ "github.com/hydroan/gst/provider/kafka"
	_ "github.com/hydroan/gst/provider/ldap"
	_ "github.com/hydroan/gst/provider/memcached"
	_ "github.com/hydroan/gst/provider/minio"
	_ "github.com/hydroan/gst/provider/mongo"
	_ "github.com/hydroan/gst/provider/mqtt"
	_ "github.com/hydroan/gst/provider/nats"
	_ "github.com/hydroan/gst/provider/rethinkdb"
	_ "github.com/hydroan/gst/provider/rocketmq"
	_ "github.com/hydroan/gst/provider/scylla"
)

// TestEveryProviderPackageSelfRegisters walks the provider/ directory and
// requires a registration for each package. It asserts containment rather
// than exact equality because unit tests in this binary register synthetic
// providers of their own.
func TestEveryProviderPackageSelfRegisters(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	entries, err := os.ReadDir(filepath.Dir(file))
	require.NoError(t, err)

	registered := make(map[string]bool)
	for _, p := range provider.Registered() {
		registered[p.Name] = true
	}

	for _, entry := range entries {
		if entry.IsDir() {
			require.True(t, registered[entry.Name()], "provider package %q must self-register under its package name", entry.Name())
		}
	}
}
