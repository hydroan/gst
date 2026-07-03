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
[wechat]
app_id = "wx123456789"

[nats]
username = "nuser"
password = "npass"
; timeout = "30s"
enabled = true
`

var filename = "/tmp/config.ini"

func TestRegisterStruct(t *testing.T) {
	if err := os.WriteFile(filename, []byte(configData), 0o644); err != nil {
		t.Fatal(err)
	}

	// Register config before bootstrap
	config.Register[Wechat]()
	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	// Register config after bootstrap
	config.Register[Nats]()

	wechat := config.Get[*Wechat]()
	assert.Equal(t, "wx123456789", wechat.AppID)
	assert.Equal(t, "myappsecret", wechat.AppSecret)
	assert.False(t, wechat.Enabled)

	nats := config.Get[Nats]()
	assert.Equal(t, "nats://127.0.0.1:4222", nats.URL)
	assert.Equal(t, "nuser", nats.Username)
	assert.Equal(t, "npass", nats.Password)
	assert.Equal(t, 5*time.Second, nats.Timeout)
	assert.True(t, nats.Enabled)
}

func TestRegisterStructPointer(t *testing.T) {
	if err := os.WriteFile(filename, []byte(configData), 0o644); err != nil {
		t.Fatal(err)
	}

	// Register config before bootstrap
	config.Register[*Wechat]()
	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	// Register config after bootstrap
	config.Register[*Nats]()
	wechat := config.Get[*Wechat]()

	assert.Equal(t, "wx123456789", wechat.AppID)
	assert.Equal(t, "myappsecret", wechat.AppSecret)
	assert.False(t, wechat.Enabled)

	nats := config.Get[Nats]()
	assert.Equal(t, "nats://127.0.0.1:4222", nats.URL)
	assert.Equal(t, "nuser", nats.Username)
	assert.Equal(t, "npass", nats.Password)
	assert.Equal(t, 5*time.Second, nats.Timeout)
	assert.True(t, nats.Enabled)
}

func TestRegisterStructFromEnv(t *testing.T) {
	if err := os.WriteFile(filename, []byte(configData), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("WECHAT_APP_SECRET", "my_app_secret")
	t.Setenv("NATS_USERNAME", "user_from_env")
	t.Setenv("NATS_PASSWORD", "pass_from_env")
	t.Setenv("NATS_TIMEOUT", "60s")

	// Register config before bootstrap
	config.Register[Wechat]()
	config.SetConfigFile(filename)
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	// Register config after bootstrap
	config.Register[Nats]()

	wechat := config.Get[*Wechat]()

	assert.Equal(t, "wx123456789", wechat.AppID)
	assert.Equal(t, "my_app_secret", wechat.AppSecret)
	assert.False(t, wechat.Enabled)

	nats := config.Get[Nats]()
	assert.Equal(t, "nats://127.0.0.1:4222", nats.URL)
	assert.Equal(t, "user_from_env", nats.Username)
	assert.Equal(t, "pass_from_env", nats.Password)
	assert.Equal(t, 60*time.Second, nats.Timeout)
	assert.True(t, nats.Enabled)
}

func TestRegisterNonStructType(t *testing.T) {
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

type Wechat struct {
	AppID     string `json:"app_id" mapstructure:"app_id" default:"myappid"`
	AppSecret string `json:"app_secret" mapstructure:"app_secret" default:"myappsecret"`
	Enabled   bool   `json:"enabled" mapstructure:"enabled"`
}

type Nats struct {
	URL      string        `json:"url" mapstructure:"url" default:"nats://127.0.0.1:4222"`
	Username string        `json:"username" mapstructure:"username" default:"nats"`
	Password string        `json:"password" mapstructure:"password" default:"nats"`
	Timeout  time.Duration `json:"timeout" mapstructure:"timeout" default:"5s"`
	Enabled  bool          `json:"enabled" mapstructure:"enabled"`
}

type TestConfig struct {
	Value string `json:"value" mapstructure:"value" default:"default_value"`
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
	}
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}
