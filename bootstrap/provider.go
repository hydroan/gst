package bootstrap

import (
	"slices"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/lifecycle"
	"go.uber.org/zap"
)

// This file holds the provider-linkage check: the configuration names the
// providers a deployment wants, the binary links the ones a project imported,
// and a name in the first list missing from the second is worth saying out
// loud while the process is still starting.

// warnUnlinkedProviders reports every provider the configuration enables
// that the binary never linked: nothing will start it, and its first use
// would fail deep inside a request rather than here. A warning rather than
// a failure, because binaries built from one project may share one
// configuration while linking different providers.
func warnUnlinkedProviders() {
	linked := make([]string, 0)
	for _, c := range lifecycle.Components(lifecycle.StageProvider) {
		linked = append(linked, c.Name)
	}
	for _, name := range unlinkedProviders(enabledProviders(), linked) {
		zap.S().Warnw("provider enabled in configuration but not compiled into this binary", "provider", name, "import", "github.com/hydroan/gst/provider/"+name)
	}
}

// enabledProviders returns the names — the package names under provider/ —
// of the providers the loaded configuration enables, to compare with the
// providers the binary linked.
//
// The clickhouse section serves two things: the provider, and the ClickHouse
// dialect when it is the primary database. With ClickHouse as the primary
// database the section enables the dialect, and no provider is expected.
func enabledProviders() []string {
	sections := []struct {
		name    string
		enabled bool
	}{
		{"cassandra", config.App.Cassandra.Enabled},
		{"clickhouse", config.App.Clickhouse.Enabled && config.App.Database.Type != config.DBClickHouse},
		{"elastic", config.App.Elasticsearch.Enabled},
		{"etcd", config.App.Etcd.Enabled},
		{"influxdb", config.App.Influxdb.Enabled},
		{"kafka", config.App.Kafka.Enabled},
		{"ldap", config.App.Ldap.Enabled},
		{"minio", config.App.Minio.Enabled},
		{"mongo", config.App.Mongo.Enabled},
		{"mqtt", config.App.Mqtt.Enabled},
		{"nats", config.App.Nats.Enabled},
		{"rethinkdb", config.App.RethinkDB.Enabled},
		{"rocketmq", config.App.RocketMQ.Enabled},
		{"scylla", config.App.Scylla.Enabled},
	}

	var names []string
	for _, section := range sections {
		if section.enabled {
			names = append(names, section.name)
		}
	}
	return names
}

// unlinkedProviders returns the enabled providers that are not linked.
func unlinkedProviders(enabled, linked []string) []string {
	var missing []string
	for _, name := range enabled {
		if !slices.Contains(linked, name) {
			missing = append(missing, name)
		}
	}
	return missing
}
