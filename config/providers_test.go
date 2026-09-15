package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnabledProvidersNamesTheEnabledSections proves the names come from the
// enabled switches alone, under the provider package names bootstrap
// compares against.
func TestEnabledProvidersNamesTheEnabledSections(t *testing.T) {
	withFreshConfig(t)

	require.Empty(t, EnabledProviders())

	App.Kafka.Enabled = true
	App.Elasticsearch.Enabled = true
	require.Equal(t, []string{"elastic", "kafka"}, EnabledProviders())
}

// TestEnabledProvidersLeaveClickhouseToThePrimaryDialect proves the
// clickhouse switch names the provider only while ClickHouse is not the
// primary database: as the primary database the switch enables the dialect,
// and a binary without the provider is in order.
func TestEnabledProvidersLeaveClickhouseToThePrimaryDialect(t *testing.T) {
	withFreshConfig(t)

	App.Clickhouse.Enabled = true
	App.Database.Type = DBClickHouse
	require.Empty(t, EnabledProviders())

	App.Database.Type = DBMySQL
	require.Equal(t, []string{"clickhouse"}, EnabledProviders())
}

// withFreshConfig points App at an empty configuration for the test and
// restores the previous one afterwards.
func withFreshConfig(t *testing.T) {
	t.Helper()

	original := App
	App = new(Config)
	t.Cleanup(func() { App = original })
}
