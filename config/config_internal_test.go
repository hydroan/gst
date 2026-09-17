package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// sampleSection is a registered section reading its fields every way a field
// can: a default tag of each kind, a list and a nested struct.
type sampleSection struct {
	Endpoint string        `mapstructure:"endpoint" default:"127.0.0.1:8080"`
	Enabled  bool          `mapstructure:"enabled" default:"true"`
	Retries  int           `mapstructure:"retries" default:"3"`
	Timeout  time.Duration `mapstructure:"timeout" default:"5s"`
	Tags     []string      `mapstructure:"tags"`
	Nested   struct {
		Label string `mapstructure:"label" default:"nested"`
	} `mapstructure:"nested"`
}

// TestInitReadsTheEnvironmentForKeysTheFileLeavesOut proves a framework key
// with no default that the file never mentions still reads its environment
// variable.
func TestInitReadsTheEnvironmentForKeysTheFileLeavesOut(t *testing.T) {
	withFreshRegistry(t, "[server]\nport = 8097\n")
	t.Setenv("AUTH_JWT_SECRET", "secret-from-env")
	t.Setenv(MYSQL_REPLICAS, "10.0.0.1:3306,10.0.0.2:3306")

	require.NoError(t, Init())

	require.Equal(t, 8097, App.Server.Port)
	require.Equal(t, "secret-from-env", App.Auth.JWTSecret)
	require.Equal(t, []string{"10.0.0.1:3306", "10.0.0.2:3306"}, App.MySQL.Replicas)
}

// TestRegisteredSectionTakesEveryEnvironmentValue proves a registered
// section's fields read their environment variables over the file and the
// default tags — an empty string, false and 0 included, a nested field and a
// list alike — and fall back to the file, then the default tags.
func TestRegisteredSectionTakesEveryEnvironmentValue(t *testing.T) {
	withFreshRegistry(t, "[sample_section]\nendpoint = file.example:8080\nretries = 7\n")
	t.Setenv("SAMPLE_SECTION_ENDPOINT", "")
	t.Setenv("SAMPLE_SECTION_ENABLED", "false")
	t.Setenv("SAMPLE_SECTION_RETRIES", "0")
	t.Setenv("SAMPLE_SECTION_TAGS", "alpha,beta")
	t.Setenv("SAMPLE_SECTION_NESTED_LABEL", "from-env")

	Register[sampleSection]()
	require.NoError(t, Init())

	section := Get[sampleSection]()
	require.Empty(t, section.Endpoint)
	require.False(t, section.Enabled)
	require.Zero(t, section.Retries)
	require.Equal(t, []string{"alpha", "beta"}, section.Tags)
	require.Equal(t, "from-env", section.Nested.Label)
	require.Equal(t, 5*time.Second, section.Timeout, "a field no variable sets keeps its default tag")
}

// TestInitFailsOnAnEnvironmentValueItCannotDecode proves a variable whose value
// does not decode into its key's type fails Init naming the variable and the
// value, for a framework key and a registered section alike, and a malformed
// default tag fails it too.
func TestInitFailsOnAnEnvironmentValueItCannotDecode(t *testing.T) {
	t.Run("framework_key", func(t *testing.T) {
		withFreshRegistry(t, "")
		t.Setenv(SERVER_PORT, "tcp://10.0.0.1:8080")

		err := Init()
		require.ErrorContains(t, err, `SERVER_PORT="tcp://10.0.0.1:8080"`)
		require.ErrorContains(t, err, "server.port")
	})

	t.Run("registered_section", func(t *testing.T) {
		withFreshRegistry(t, "")
		t.Setenv("SAMPLE_SECTION_TIMEOUT", "5")

		Register[sampleSection]()
		require.ErrorContains(t, Init(), `SAMPLE_SECTION_TIMEOUT="5"`)
	})

	t.Run("malformed_default_tag", func(t *testing.T) {
		withFreshRegistry(t, "")
		type malformedDefault struct {
			Timeout time.Duration `mapstructure:"timeout" default:"soon"`
		}

		Register[malformedDefault]()
		require.ErrorContains(t, Init(), `default tag "soon"`)
	})
}

// TestRegisterRefusesATakenSection proves a registered section cannot take a
// section of the framework's own configuration or of another registered type:
// a registration made before Init fails Init, once, and leaves the section
// out, and one made after Init panics.
func TestRegisterRefusesATakenSection(t *testing.T) {
	t.Run("framework_section", func(t *testing.T) {
		withFreshRegistry(t, "")
		type Mysql struct {
			Host string `mapstructure:"host"`
		}

		Register[Mysql]()
		require.ErrorContains(t, Init(), `section "mysql"`)
		require.NoError(t, Init(), "a refused registration is reported once")
		require.Panics(t, func() { Register[Mysql]() })
	})

	t.Run("two_types", func(t *testing.T) {
		withFreshRegistry(t, "")
		registerFirst := func() {
			type Duplicate struct {
				Label string `mapstructure:"label"`
			}
			Register[Duplicate]()
		}
		registerSecond := func() {
			type Duplicate struct {
				Count int `mapstructure:"count"`
			}
			Register[Duplicate]()
		}

		registerFirst()
		registerFirst()
		registerSecond()
		require.ErrorContains(t, Init(), `both register section "duplicate"`)
		require.NoError(t, Init(), "a refused registration is reported once")
		require.Panics(t, registerSecond)
	})
}

// withFreshRegistry runs a test as a process that has registered nothing and
// not run Init yet, reading a config file with content, and puts the package
// state back after.
func withFreshRegistry(t *testing.T, content string) {
	t.Helper()

	file := filepath.Join(t.TempDir(), "config.ini")
	require.NoError(t, os.WriteFile(file, []byte(content), 0o600))

	mu.Lock()
	savedTypes, savedConfigs, savedErr, savedInitialized := registeredTypes, registeredConfigs, errRegister, initialized
	savedApp, savedViper, savedFile := App, cv, configFile
	registeredTypes = make(map[string]reflect.Type)
	registeredConfigs = make(map[string]any)
	errRegister = nil
	initialized = false
	configFile = file
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		registeredTypes, registeredConfigs, errRegister, initialized = savedTypes, savedConfigs, savedErr, savedInitialized
		App, cv, configFile = savedApp, savedViper, savedFile
	})
}
