package config_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestEnvironmentCarriesATimeAndAMap pins the two kinds of value an
// environment variable could not carry: the framework's own build time, and
// the tag map of the metrics client. Both are ordinary configuration keys, so
// a deployment that configures everything through the environment — a
// container, a k8s pod — must be able to set them there.
func TestEnvironmentCarriesATimeAndAMap(t *testing.T) {
	t.Setenv(config.APP_BUILD_TIME, "2026-07-01T08:30:15+08:00")
	t.Setenv(config.INFLUXDB_DEFAULT_TAGS, "env=prod, region=cn-south")

	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, "[app]\nname = \"sample\"\n")
	config.SetConfigFile(filename)
	require.NoError(t, config.Init())

	require.Equal(t,
		time.Date(2026, 7, 1, 8, 30, 15, 0, time.FixedZone("UTC+8", 8*3600)).UTC(),
		config.App.AppInfo.BuildTime.UTC(),
		"a time arrives in RFC 3339, the one format the framework reads from outside")
	require.Equal(t, map[string]string{"env": "prod", "region": "cn-south"}, config.App.Influxdb.DefaultTags,
		"a map arrives as key=value pairs, the one spelling a single variable can carry")
}

// TestMalformedMapEnvironmentValueNamesTheVariable pins the other half: a
// value that is not a map fails at startup naming the variable, instead of
// reaching the client as an empty tag set.
func TestMalformedMapEnvironmentValueNamesTheVariable(t *testing.T) {
	t.Setenv(config.INFLUXDB_DEFAULT_TAGS, "env:prod")

	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, "[app]\nname = \"sample\"\n")
	config.SetConfigFile(filename)
	err := config.Init()
	require.ErrorContains(t, err, config.INFLUXDB_DEFAULT_TAGS)
}

// nestedDefaults is a section whose values sit behind a pointer, the shape a
// project reaches for when a group of settings is optional.
type nestedDefaults struct {
	Endpoint string       `json:"endpoint" mapstructure:"endpoint" default:"127.0.0.1:9000"`
	TLS      *nestedTLS   `json:"tls" mapstructure:"tls"`
	Retry    *nestedRetry `json:"retry" mapstructure:"retry"`
}

type nestedTLS struct {
	CertFile string `json:"cert_file" mapstructure:"cert_file" default:"/etc/gst/tls.crt"`
	Enabled  bool   `json:"enabled" mapstructure:"enabled" default:"true"`
}

type nestedRetry struct {
	Attempts int           `json:"attempts" mapstructure:"attempts" default:"3"`
	Interval time.Duration `json:"interval" mapstructure:"interval" default:"2s"`
}

// TestSectionDefaultsReachBehindNilPointers pins that a default tag means the
// same wherever the field sits. Skipping the tags behind a nil pointer would
// bring a section declaring values up with zeros, and the project would only
// find out when the empty value reached whatever reads it.
func TestSectionDefaultsReachBehindNilPointers(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, "[app]\nname = \"sample\"\n")
	config.SetConfigFile(filename)
	require.NoError(t, config.Init())

	config.Register[nestedDefaults]()
	section := config.Get[*nestedDefaults]()
	require.Equal(t, "127.0.0.1:9000", section.Endpoint)
	require.NotNil(t, section.TLS, "a nil pointer is filled in, not left for the reader to check")
	require.Equal(t, "/etc/gst/tls.crt", section.TLS.CertFile)
	require.True(t, section.TLS.Enabled)
	require.NotNil(t, section.Retry)
	require.Equal(t, 3, section.Retry.Attempts)
	require.Equal(t, 2*time.Second, section.Retry.Interval)
}

// LoggerHTTP is a section whose name and key together spell
// LOGGER_HTTP_BODY_ENABLED, the variable the framework's own
// logger.http_body.enabled key reads.
type LoggerHTTP struct {
	BodyEnabled bool `json:"body_enabled" mapstructure:"body_enabled"`
}

// TestSectionRefusesAKeySharingAVariable pins the refusal: one environment
// variable setting two keys is a deployment that cannot say which it meant,
// and the value would land in both. The clash is in the variable, not in the
// section name — "logger_http" is free, and its key still collides.
func TestSectionRefusesAKeySharingAVariable(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, "[app]\nname = \"sample\"\n")
	config.SetConfigFile(filename)
	require.NoError(t, config.Init())

	require.PanicsWithError(t,
		`config: config_test.LoggerHTTP registers key "logger_http.body_enabled", which reads the same environment variable LOGGER_HTTP_BODY_ENABLED as "logger.http_body.enabled"`,
		func() { config.Register[LoggerHTTP]() },
		"a section registered after Init reports the clash by panicking, the way every other registration failure does")
}

// TestDurationWrittenAsANumberIsRefused pins that a duration needs its unit in
// every file format: a bare number in YAML, JSON or TOML arrives as a number,
// which would otherwise decode as nanoseconds and leave a timeout of "15" at
// fifteen nanoseconds without a word; an INI file hands the same text over as
// a string, which the string hook refuses already. Zero alone carries no unit.
func TestDurationWrittenAsANumberIsRefused(t *testing.T) {
	clearConfigEnvForTest(t)
	for _, tt := range []struct {
		format string
		bare   string
		unit   string
		zero   string
	}{
		{format: "yaml", bare: "server:\n  read_timeout: 15\n", unit: "server:\n  read_timeout: 15s\n", zero: "server:\n  read_timeout: 0\n"},
		{format: "json", bare: `{"server": {"read_timeout": 15}}`, unit: `{"server": {"read_timeout": "15s"}}`, zero: `{"server": {"read_timeout": 0}}`},
		{format: "toml", bare: "[server]\nread_timeout = 15\n", unit: "[server]\nread_timeout = \"15s\"\n", zero: "[server]\nread_timeout = 0\n"},
		{format: "ini", bare: "[server]\nread_timeout = 15\n", unit: "[server]\nread_timeout = 15s\n", zero: "[server]\nread_timeout = 0\n"},
	} {
		t.Run(tt.format, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "config."+tt.format)
			config.SetConfigFile(filename)

			requireWriteConfigFile(t, filename, tt.bare)
			err := config.Init()
			require.Error(t, err, "a duration without a unit must be refused, not read as nanoseconds")
			require.ErrorContains(t, err, "read_timeout", "the refusal names the key")
			require.ErrorContains(t, err, "unit")

			requireWriteConfigFile(t, filename, tt.unit)
			require.NoError(t, config.Init())
			require.Equal(t, 15*time.Second, config.App.Server.ReadTimeout)

			requireWriteConfigFile(t, filename, tt.zero)
			require.NoError(t, config.Init(), "zero carries no unit, as it carries none in time.ParseDuration")
			require.Equal(t, time.Duration(0), config.App.Server.ReadTimeout)
		})
	}
}

// TestValueOutsideItsSetIsRefused pins the decode of every configuration
// value whose type admits a fixed set of spellings, the closed-set types of
// the framework's sections: text outside the set fails Init naming the key
// and quoting the text, from the environment and from a file alike, and text
// inside the set is read as the spelling the type writes, whatever its letter
// case and surrounding space. Every section gets the same refusal; none reads
// a default of its own in place of text it does not know.
func TestValueOutsideItsSetIsRefused(t *testing.T) {
	cases := []struct {
		variable string
		key      string
		outside  string
		inside   string
		want     string
		read     func() string
	}{
		{config.DATABASE_TYPE, "database.type", "oracle", " MySQL ", "mysql", func() string { return string(config.App.Database.Type) }},
		{config.LOGGER_OUTPUT, "logger.output", "syslog", "File", "file", func() string { return string(config.App.Logger.Output) }},
		{config.MONGO_READ_CONCERN, "mongo.read_concern", "eventual", "Majority", "majority", func() string { return string(config.App.Mongo.ReadConcern) }},
		{config.MONGO_WRITE_CONCERN, "mongo.write_concern", "-1", " Journaled ", "journaled", func() string { return string(config.App.Mongo.WriteConcern) }},
		{config.SCYLLA_CONSISTENCY, "scylla.consistency", "most", "LOCAL_QUORUM", "local_quorum", func() string { return string(config.App.Scylla.Consistency) }},
		{config.SCYLLA_RETRY_POLICY, "scylla.retry_policy", "never", "Simple", "simple", func() string { return string(config.App.Scylla.RetryPolicy) }},
		{config.SCYLLA_RECONNECT_POLICY, "scylla.reconnect_policy", "linear", "Exponential", "exponential", func() string { return string(config.App.Scylla.ReconnectPolicy) }},
		{config.ROCKETMQ_CONSUME_FROM_WHERE, "rocketmq.consume_from_where", "middle", "First_Offset", "first_offset", func() string { return string(config.App.RocketMQ.ConsumeFromWhere) }},
		{config.OTEL_EXPORTER_OTLP_PROTOCOL, "otel.exporter_otlp_protocol", "http/json", "GRPC", "grpc", func() string { return string(config.App.OTEL.ExporterOTLPProtocol) }},
		{config.OTEL_EXPORTER_OTLP_COMPRESSION, "otel.exporter_otlp_compression", "brotli", "Gzip", "gzip", func() string { return string(config.App.OTEL.ExporterOTLPCompression) }},
		{config.OTEL_TRACES_SAMPLER, "otel.traces_sampler", "sometimes", "Always_On", "always_on", func() string { return string(config.App.OTEL.TracesSampler) }},
	}
	for _, tt := range cases {
		t.Run(tt.key, func(t *testing.T) {
			clearConfigEnvForTest(t)
			filename := filepath.Join(t.TempDir(), "config.ini")
			requireWriteConfigFile(t, filename, "[app]\nname = \"sample\"\n")
			config.SetConfigFile(filename)

			t.Setenv(tt.variable, tt.outside)
			err := config.Init()
			require.Error(t, err, "text outside the set must be refused, not read as a default")
			require.ErrorContains(t, err, tt.key, "the refusal names the key")
			require.ErrorContains(t, err, tt.outside, "the refusal quotes the text")

			t.Setenv(tt.variable, tt.inside)
			require.NoError(t, config.Init())
			require.Equal(t, tt.want, tt.read(), "text inside the set reads as the spelling the type writes")
		})
	}

	t.Run("file", func(t *testing.T) {
		clearConfigEnvForTest(t)
		filename := filepath.Join(t.TempDir(), "config.yaml")
		config.SetConfigFile(filename)

		requireWriteConfigFile(t, filename, "database:\n  type: oracle\n")
		err := config.Init()
		require.Error(t, err, "a file is held to the set the same as the environment")
		require.ErrorContains(t, err, "database.type")
		require.ErrorContains(t, err, "oracle")

		requireWriteConfigFile(t, filename, "database:\n  type: Postgres\n")
		require.NoError(t, config.Init())
		require.Equal(t, config.DBPostgres, config.App.Database.Type)
	})
}

// TestWriteConcernAdmitsNumbersAndTagNames pins the one set open at one end:
// mongo.write_concern names majority or journaled, a number of acknowledging
// members, or a write concern the replica set itself defines, whose name is
// kept as written. Only a number that is negative or is not one is refused.
func TestWriteConcernAdmitsNumbersAndTagNames(t *testing.T) {
	clearConfigEnvForTest(t)
	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, "[app]\nname = \"sample\"\n")
	config.SetConfigFile(filename)

	for _, tt := range []struct{ text, want string }{
		{" 3 ", "3"},
		{"myRegion", "myRegion"},
	} {
		t.Setenv(config.MONGO_WRITE_CONCERN, tt.text)
		require.NoError(t, config.Init(), tt.text)
		require.Equal(t, config.WriteConcern(tt.want), config.App.Mongo.WriteConcern)
	}
	for _, text := range []string{"1x", "-2"} {
		t.Setenv(config.MONGO_WRITE_CONCERN, text)
		err := config.Init()
		require.Error(t, err, text)
		require.ErrorContains(t, err, "mongo.write_concern")
	}
}
