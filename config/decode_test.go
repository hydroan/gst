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
// same wherever the field sits. The tags behind a nil pointer used to be
// skipped for everything but a duration, so a section declaring values came
// up with zeros and the project only found out when the empty value reached
// whatever reads it.
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
