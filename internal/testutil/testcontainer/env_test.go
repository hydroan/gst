package testcontainer

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

func TestApplyConfigToEnv(t *testing.T) {
	t.Run("flat_section", func(t *testing.T) {
		isolateEnv(t,
			config.MYSQL_HOST, config.MYSQL_PORT, config.MYSQL_DATABASE,
			config.MYSQL_USERNAME, config.MYSQL_PASSWORD, config.MYSQL_DIAL_TIMEOUT)

		host := "localhost"
		port := 3306
		database := "test"
		username := "test"
		password := "test"

		ApplyConfigToEnv(config.MySQL{
			Host:     host,
			Port:     uint(port),
			Database: database,
			Username: username,
			Password: password,
		})

		require.Equal(t, host, os.Getenv(config.MYSQL_HOST))
		require.Equal(t, strconv.Itoa(port), os.Getenv(config.MYSQL_PORT))
		require.Equal(t, database, os.Getenv(config.MYSQL_DATABASE))
		require.Equal(t, username, os.Getenv(config.MYSQL_USERNAME))
		require.Equal(t, password, os.Getenv(config.MYSQL_PASSWORD))
		// DialTimeout was left at its zero value, so the framework default
		// stands.
		require.Empty(t, os.Getenv(config.MYSQL_DIAL_TIMEOUT))
	})

	t.Run("nested_section", func(t *testing.T) {
		isolateEnv(t,
			config.LOGGER_PREFIX, config.LOGGER_DIR, config.LOGGER_CONSOLE,
			config.LOGGER_MAX_AGE, config.LOGGER_HTTP_BODY_ENABLED,
			config.LOGGER_HTTP_BODY_LOG_REQUEST, config.LOGGER_HTTP_BODY_LOG_RESPONSE,
			config.LOGGER_HTTP_BODY_MAX_BODY_SIZE, config.LOGGER_HTTP_BODY_SKIP_ROUTES)

		ApplyConfigToEnv(config.Logger{
			Prefix:  "test",
			Console: false,
			MaxAge:  7,
			HTTPBody: config.HTTPBodyLogger{
				Enabled:     true,
				LogRequest:  config.HTTPBodyLogModeAll,
				LogResponse: config.HTTPBodyLogModeError,
				MaxBodySize: "64KB",
				SkipRoutes:  []string{"/api/login"},
			},
		})

		require.Equal(t, "test", os.Getenv(config.LOGGER_PREFIX))
		require.Equal(t, "7", os.Getenv(config.LOGGER_MAX_AGE))
		// A nested section is named after its field, not after its type.
		require.Equal(t, "true", os.Getenv(config.LOGGER_HTTP_BODY_ENABLED))
		require.Equal(t, string(config.HTTPBodyLogModeAll), os.Getenv(config.LOGGER_HTTP_BODY_LOG_REQUEST))
		require.Equal(t, string(config.HTTPBodyLogModeError), os.Getenv(config.LOGGER_HTTP_BODY_LOG_RESPONSE))
		require.Equal(t, "64KB", os.Getenv(config.LOGGER_HTTP_BODY_MAX_BODY_SIZE))

		require.Empty(t, os.Getenv(config.LOGGER_DIR))
		// A false bool is indistinguishable from an unset one, so it is left
		// out rather than overriding a framework default of true.
		require.Empty(t, os.Getenv(config.LOGGER_CONSOLE))
		// A slice has no single environment representation.
		require.Empty(t, os.Getenv(config.LOGGER_HTTP_BODY_SKIP_ROUTES))
	})

	t.Run("duration_field", func(t *testing.T) {
		isolateEnv(t, config.DATABASE_SLOW_QUERY_THRESHOLD)

		ApplyConfigToEnv(config.Database{SlowQueryThreshold: 500 * time.Millisecond})

		require.Equal(t, "500ms", os.Getenv(config.DATABASE_SLOW_QUERY_THRESHOLD))
	})

	t.Run("section_named_differently_from_its_type", func(t *testing.T) {
		isolateEnv(t, config.APP_NAME)

		ApplyConfigToEnv(config.AppInfo{Name: "sample"})

		require.Equal(t, "sample", os.Getenv(config.APP_NAME))
	})

	t.Run("pointer_section", func(t *testing.T) {
		isolateEnv(t, config.SQLITE_PATH)

		ApplyConfigToEnv(&config.Sqlite{Path: "/tmp/sample.db"})

		require.Equal(t, "/tmp/sample.db", os.Getenv(config.SQLITE_PATH))
	})

	t.Run("value_without_fields", func(t *testing.T) {
		require.NotPanics(t, func() {
			ApplyConfigToEnv(nil)
			ApplyConfigToEnv(42)
			ApplyConfigToEnv((*config.Sqlite)(nil))
		})
	})
}

// isolateEnv restores keys to whatever they held before the test, so that a
// case asserting on an unset variable is not fooled by a value another case or
// the surrounding environment left behind.
func isolateEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}
