package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/assert"
)

var configData = `
[sample]
app_id = "wx123456789"

[relay]
username = "nuser"
password = "npass"
; timeout = "30s"
enabled = true
`

func TestRegisterReadsTheFileOverTheDefaultTags(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, configData)

	// Register config before Init
	config.Register[Sample]()
	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	// Register config after Init
	config.Register[Relay]()

	sample := config.Get[*Sample]()
	assert.Equal(t, "wx123456789", sample.AppID)
	assert.Equal(t, "myappsecret", sample.AppSecret)
	assert.False(t, sample.Enabled)

	relay := config.Get[Relay]()
	assert.Equal(t, "tcp://127.0.0.1:4222", relay.URL)
	assert.Equal(t, "nuser", relay.Username)
	assert.Equal(t, "npass", relay.Password)
	assert.Equal(t, 5*time.Second, relay.Timeout)
	assert.True(t, relay.Enabled)
}

func TestRegisterTakesAPointerType(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, configData)

	// Register config before Init
	config.Register[*Sample]()
	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	// Register config after Init
	config.Register[*Relay]()
	sample := config.Get[*Sample]()

	assert.Equal(t, "wx123456789", sample.AppID)
	assert.Equal(t, "myappsecret", sample.AppSecret)
	assert.False(t, sample.Enabled)

	relay := config.Get[Relay]()
	assert.Equal(t, "tcp://127.0.0.1:4222", relay.URL)
	assert.Equal(t, "nuser", relay.Username)
	assert.Equal(t, "npass", relay.Password)
	assert.Equal(t, 5*time.Second, relay.Timeout)
	assert.True(t, relay.Enabled)
}

func TestRegisterReadsTheEnvironmentOverTheFile(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, configData)

	t.Setenv("SAMPLE_APP_SECRET", "my_app_secret")
	t.Setenv("RELAY_USERNAME", "user_from_env")
	t.Setenv("RELAY_PASSWORD", "pass_from_env")
	t.Setenv("RELAY_TIMEOUT", "60s")

	// Register config before Init
	config.Register[Sample]()
	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	// Register config after Init
	config.Register[Relay]()

	sample := config.Get[*Sample]()

	assert.Equal(t, "wx123456789", sample.AppID)
	assert.Equal(t, "my_app_secret", sample.AppSecret)
	assert.False(t, sample.Enabled)

	relay := config.Get[Relay]()
	assert.Equal(t, "tcp://127.0.0.1:4222", relay.URL)
	assert.Equal(t, "user_from_env", relay.Username)
	assert.Equal(t, "pass_from_env", relay.Password)
	assert.Equal(t, 60*time.Second, relay.Timeout)
	assert.True(t, relay.Enabled)
}

func TestRegisterSkipsANonStructType(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, configData)

	// These should be skipped silently without error or panic
	config.Register[string]()
	config.Register[int]()
	config.Register[*string]()
	config.Register[[]string]()
	config.Register[map[string]string]()

	// Should not panic or cause errors
	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	// Getting non-registered configs should return zero values
	strVal := config.Get[string]()
	assert.Empty(t, strVal)

	intVal := config.Get[int]()
	assert.Equal(t, 0, intVal)
}

// TestLoggerSQLCallerSkipPrefixesFromEnv pins the environment form of the
// logger.sql_caller_skip_prefixes list: built-in sections unmarshal through
// viper, whose default decode hook splits an environment string on commas.
func TestLoggerSQLCallerSkipPrefixesFromEnv(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.ini")
	requireWriteConfigFile(t, filename, configData)

	t.Setenv("LOGGER_SQL_CALLER_SKIP_PREFIXES", "example.com/app/dao,example.com/app/repo")

	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t,
		[]string{"example.com/app/dao", "example.com/app/repo"},
		config.App.Logger.SQLCallerSkipPrefixes)
}

func TestInitReadsYAMLConfigFile(t *testing.T) {
	clearConfigEnvForTest(t)

	filename := filepath.Join(t.TempDir(), "config.yaml")
	requireWriteConfigFile(t, filename, `
server:
  port: 8091
  mode: test
redis:
  enabled: true
  namespace: yamlapp
`)

	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 8091, config.App.Server.Port)
	assert.Equal(t, config.Mode("test"), config.App.Server.Mode)
	assert.True(t, config.App.Redis.Enabled)
	assert.Equal(t, "yamlapp", config.App.Redis.Namespace)
}

func TestInitReadsJSONConfigFile(t *testing.T) {
	clearConfigEnvForTest(t)

	filename := filepath.Join(t.TempDir(), "config.json")
	requireWriteConfigFile(t, filename, `{
  "server": {
    "port": 8092,
    "mode": "local"
  },
  "redis": {
    "enabled": true,
    "namespace": "jsonapp"
  }
}`)

	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 8092, config.App.Server.Port)
	assert.Equal(t, config.Mode("local"), config.App.Server.Mode)
	assert.True(t, config.App.Redis.Enabled)
	assert.Equal(t, "jsonapp", config.App.Redis.Namespace)
}

func TestInitReadsTOMLConfigFile(t *testing.T) {
	clearConfigEnvForTest(t)

	filename := filepath.Join(t.TempDir(), "config.toml")
	requireWriteConfigFile(t, filename, `
[server]
port = 8095
mode = "stg"

[redis]
enabled = true
namespace = "tomlapp"
`)

	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 8095, config.App.Server.Port)
	assert.Equal(t, config.Mode("stg"), config.App.Server.Mode)
	assert.True(t, config.App.Redis.Enabled)
	assert.Equal(t, "tomlapp", config.App.Redis.Namespace)
}

func TestInitDiscoversYAMLConfigByDefault(t *testing.T) {
	clearConfigEnvForTest(t)
	t.Chdir(t.TempDir())
	requireWriteConfigFile(t, "config.yaml", `
server:
  port: 8093
  mode: pre
`)

	config.SetConfigFile("")
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 8093, config.App.Server.Port)
	assert.Equal(t, config.Mode("pre"), config.App.Server.Mode)
}

func TestInitDiscoversTOMLConfigByDefault(t *testing.T) {
	clearConfigEnvForTest(t)
	t.Chdir(t.TempDir())
	requireWriteConfigFile(t, "config.toml", `
[server]
port = 8096
mode = "prod"
`)

	config.SetConfigFile("")
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 8096, config.App.Server.Port)
	assert.Equal(t, config.Mode("prod"), config.App.Server.Mode)
}

// TestInitDefaultsToInMemorySqliteWithoutConfigFile pins the defaults a missing
// config file falls back to. Table preparation exempts this combination from
// the "gg migrate" requirement, so the two packages must agree on it.
func TestInitDefaultsToInMemorySqliteWithoutConfigFile(t *testing.T) {
	clearConfigEnvForTest(t)
	t.Chdir(t.TempDir())

	config.SetConfigFile("")
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, config.DBSqlite, config.App.Database.Type)
	assert.True(t, config.App.Sqlite.IsMemory)
	assert.False(t, config.App.Database.AutoMigrate)
}

// Sample is a registered section read from the file and its default tags.
type Sample struct {
	AppID     string `json:"app_id" mapstructure:"app_id" default:"myappid"`
	AppSecret string `json:"app_secret" mapstructure:"app_secret" default:"myappsecret"`
	Enabled   bool   `json:"enabled" mapstructure:"enabled"`
}

// Relay is a registered section read after Init.
type Relay struct {
	URL      string        `json:"url" mapstructure:"url" default:"tcp://127.0.0.1:4222"`
	Username string        `json:"username" mapstructure:"username" default:"relay"`
	Password string        `json:"password" mapstructure:"password" default:"relay"`
	Timeout  time.Duration `json:"timeout" mapstructure:"timeout" default:"5s"`
	Enabled  bool          `json:"enabled" mapstructure:"enabled"`
}

func requireWriteConfigFile(t *testing.T, filename, content string) {
	t.Helper()

	if dir := filepath.Dir(filename); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func clearConfigEnvForTest(t *testing.T) {
	t.Helper()

	keys := []string{
		"SERVER_MODE",
		"SERVER_PORT",
		"REDIS_ENABLED",
		"REDIS_NAMESPACE",
		config.DATABASE_TYPE,
		config.DATABASE_AUTO_MIGRATE,
		config.SQLITE_IS_MEMORY,
	}
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}
