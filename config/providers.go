package config

// EnabledProviders returns the names — the package names under provider/ —
// of the providers the loaded configuration enables. Bootstrap compares them
// with the providers the binary linked: one enabled here that no package
// imported would silently never start, and fail at its first use instead.
//
// The clickhouse section serves two things: the provider, and the ClickHouse
// dialect when it is the primary database. With ClickHouse as the primary
// database the section enables the dialect, and no provider is expected.
func EnabledProviders() []string {
	sections := []struct {
		name    string
		enabled bool
	}{
		{"cassandra", App.Cassandra.Enabled},
		{"clickhouse", App.Clickhouse.Enabled && App.Database.Type != DBClickHouse},
		{"elastic", App.Elasticsearch.Enabled},
		{"etcd", App.Etcd.Enabled},
		{"influxdb", App.Influxdb.Enabled},
		{"kafka", App.Kafka.Enabled},
		{"ldap", App.Ldap.Enabled},
		{"minio", App.Minio.Enabled},
		{"mongo", App.Mongo.Enabled},
		{"mqtt", App.Mqtt.Enabled},
		{"nats", App.Nats.Enabled},
		{"rethinkdb", App.RethinkDB.Enabled},
		{"rocketmq", App.RocketMQ.Enabled},
		{"scylla", App.Scylla.Enabled},
	}

	var names []string
	for _, section := range sections {
		if section.enabled {
			names = append(names, section.name)
		}
	}
	return names
}
