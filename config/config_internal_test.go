package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// sampleSection is a registered section reading its fields every way a field
// can: a default tag of each kind, a list, a nested struct, an unexported
// struct squashed in, a time and a pointer to one, a field no key reads, and
// fields whose tags package defaults passes over.
type sampleSection struct {
	sampleBase `mapstructure:",squash"`

	Endpoint string        `mapstructure:"endpoint" default:"127.0.0.1:8080"`
	Enabled  bool          `mapstructure:"enabled" default:"true"`
	Retries  int           `mapstructure:"retries" default:"3"`
	Timeout  time.Duration `mapstructure:"timeout" default:"5s"`
	Tags     []string      `mapstructure:"tags"`
	Nested   struct {
		Label string `mapstructure:"label" default:"nested"`
	} `mapstructure:"nested"`
	Since    time.Time     `mapstructure:"since"`
	Until    *time.Time    `mapstructure:"until"`
	Internal string        `mapstructure:"-" default:"internal"`
	Unset    time.Duration `mapstructure:"unset" default:"-"`
	Blank    time.Duration `mapstructure:"blank" default:""`
	Skipped  struct {
		Enabled bool `mapstructure:"enabled" default:"yes"`
	} `mapstructure:"skipped" default:"-"`
}

// sampleBase is squashed into sampleSection unexported: its fields are keys
// of the section all the same.
type sampleBase struct {
	Region string `mapstructure:"region"`
}

// checkedSection is a registered section that checks its own values, its
// Validate declared on the struct.
type checkedSection struct {
	Workers int `mapstructure:"workers" default:"4"`
}

func (c checkedSection) Validate() error {
	if c.Workers < 1 {
		return errors.Newf("workers must be at least 1, got %d", c.Workers)
	}
	return nil
}

// checkedPointerSection is a registered section whose Validate is declared on
// a pointer to the struct.
type checkedPointerSection struct {
	Workers int `mapstructure:"workers" default:"4"`
}

func (c *checkedPointerSection) Validate() error {
	if c.Workers < 1 {
		return errors.Newf("workers must be at least 1, got %d", c.Workers)
	}
	return nil
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

// TestInitReadsMySQLStrictFromTheEnvironment proves the strict switch, on
// by default, reads its environment variable.
func TestInitReadsMySQLStrictFromTheEnvironment(t *testing.T) {
	withFreshRegistry(t, "")
	t.Setenv(MYSQL_STRICT, "false")

	require.NoError(t, Init())

	require.False(t, App.MySQL.Strict)
}

// TestInitReadsMySQLTLSFromTheEnvironment proves the TLS keys of the mysql
// section, off and empty by default, read their environment variables.
func TestInitReadsMySQLTLSFromTheEnvironment(t *testing.T) {
	withFreshRegistry(t, "")
	require.NoError(t, Init())
	require.False(t, App.MySQL.TLSEnabled)
	require.Empty(t, App.MySQL.CertFile)
	require.Empty(t, App.MySQL.KeyFile)
	require.Empty(t, App.MySQL.CAFile)
	require.False(t, App.MySQL.InsecureSkipVerify)

	t.Setenv(MYSQL_TLS_ENABLED, "true")
	t.Setenv(MYSQL_CERT_FILE, "client-cert.pem")
	t.Setenv(MYSQL_KEY_FILE, "client-key.pem")
	t.Setenv(MYSQL_CA_FILE, "ca.pem")
	t.Setenv(MYSQL_INSECURE_SKIP_VERIFY, "true")
	require.NoError(t, Init())
	require.True(t, App.MySQL.TLSEnabled)
	require.Equal(t, "client-cert.pem", App.MySQL.CertFile)
	require.Equal(t, "client-key.pem", App.MySQL.KeyFile)
	require.Equal(t, "ca.pem", App.MySQL.CAFile)
	require.True(t, App.MySQL.InsecureSkipVerify)
}

// TestRegisteredSectionTakesEveryEnvironmentValue proves a registered
// section's fields read their environment variables over the file and the
// default tags — an empty string, false and 0 included, a nested field, a list
// and a field of a squashed unexported struct alike — and fall back to the
// file, then the default tags; a tag package defaults passes over fails
// nothing.
func TestRegisteredSectionTakesEveryEnvironmentValue(t *testing.T) {
	withFreshRegistry(t, "[sample_section]\nendpoint = file.example:8080\nretries = 7\nregion = file-region\n")
	t.Setenv("SAMPLE_SECTION_ENDPOINT", "")
	t.Setenv("SAMPLE_SECTION_ENABLED", "false")
	t.Setenv("SAMPLE_SECTION_RETRIES", "0")
	t.Setenv("SAMPLE_SECTION_TAGS", "alpha,beta")
	t.Setenv("SAMPLE_SECTION_NESTED_LABEL", "from-env")
	t.Setenv("SAMPLE_SECTION_REGION", "env-region")

	Register[sampleSection]()
	require.NoError(t, Init())

	section := Get[sampleSection]()
	require.Empty(t, section.Endpoint)
	require.False(t, section.Enabled)
	require.Zero(t, section.Retries)
	require.Equal(t, []string{"alpha", "beta"}, section.Tags)
	require.Equal(t, "from-env", section.Nested.Label)
	require.Equal(t, "env-region", section.Region, "a field of a squashed unexported struct reads its variable too")
	require.Equal(t, 5*time.Second, section.Timeout, "a field no variable sets keeps its default tag")
	require.Equal(t, "internal", section.Internal, "a field no key reads keeps its default tag")
	require.True(t, section.Since.IsZero(), "a time is a value of its own, not a nested section")
	require.Nil(t, section.Until, "a pointer to a time nothing sets stays nil")
	require.Zero(t, section.Unset)
	require.Zero(t, section.Blank)
	require.False(t, section.Skipped.Enabled, "a struct tagged to be passed over keeps its zero value")
}

// TestVariableNamingNoKeyHasNoEffect proves a variable that names no key
// reads nothing and fails nothing: one named like a section rather than a key
// — a framework section, a registered section or a nested struct in one — and
// one named after a key no section has, under a framework section, a
// registered section or a section nothing knows. A process shares its
// environment with everything else running in it, so neither Init nor
// CheckUnknownSections holds a variable against the configuration, and the
// keys still resolve from their own variables, the file and the defaults.
func TestVariableNamingNoKeyHasNoEffect(t *testing.T) {
	withFreshRegistry(t, "[server]\nport = 8097\n[sample_section]\nretries = 7\n")
	t.Setenv("SERVER", "prod")
	t.Setenv("SAMPLE_SECTION", "")
	t.Setenv("SAMPLE_SECTION_NESTED", "on")
	t.Setenv("SERVER_NOSUCHKEY", "1")
	t.Setenv("SAMPLE_SECTION_NOSUCHKEY", "1")
	t.Setenv("REIDS_ADDR", "127.0.0.1:6379")
	t.Setenv("NOSUCHKEY", "1")

	Register[sampleSection]()
	require.NoError(t, Init())
	require.NoError(t, CheckUnknownSections())

	require.Equal(t, 8097, App.Server.Port)
	section := Get[sampleSection]()
	require.Equal(t, 7, section.Retries)
	require.Equal(t, "127.0.0.1:8080", section.Endpoint)
	require.Equal(t, "nested", section.Nested.Label)
}

// TestInitRefusesAKeyTheRegisteredSectionLacks pins that a key the file
// carries under a registered section that no field of its type reads —
// misspelled, or under a nested group — fails Init the way one under a
// section of the framework's own does, naming the section and the key, every
// such key at once in sorted order.
func TestInitRefusesAKeyTheRegisteredSectionLacks(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
	}{
		{"misspelled_key", "[sample_section]\nendpiont = file.example:8080\n", `config: section "sample_section" has no key "endpiont"`},
		{"nested_group", "[sample_section.nested]\nlabell = named\n", `config: section "sample_section" has no key "nested.labell"`},
		{"every_unknown_key_in_order", "[sample_section]\nzone = a\nendpiont = b\n", `config: section "sample_section" has no key "endpiont"; section "sample_section" has no key "zone"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withFreshRegistry(t, tc.content)

			Register[sampleSection]()
			require.EqualError(t, Init(), tc.want)
		})
	}
}

// TestInitRunsTheValidateOfARegisteredSection pins the optional Validate
// method of a registered section, declared on the struct or on a pointer to
// it: it is called on the section as loaded, from the file, the environment
// and the default tags, and an error it returns fails Init naming the section
// and its type; a section whose values pass loads as any other.
func TestInitRunsTheValidateOfARegisteredSection(t *testing.T) {
	t.Run("refused_from_the_file", func(t *testing.T) {
		withFreshRegistry(t, "[checked_section]\nworkers = 0\n")

		Register[checkedSection]()
		require.EqualError(t, Init(), `config: section "checked_section" of config.checkedSection: workers must be at least 1, got 0`)
	})

	t.Run("refused_from_the_environment", func(t *testing.T) {
		withFreshRegistry(t, "[checked_pointer_section]\nworkers = 8\n")
		t.Setenv("CHECKED_POINTER_SECTION_WORKERS", "-2")

		Register[checkedPointerSection]()
		require.EqualError(t, Init(), `config: section "checked_pointer_section" of config.checkedPointerSection: workers must be at least 1, got -2`)
	})

	t.Run("accepted", func(t *testing.T) {
		withFreshRegistry(t, "[checked_section]\nworkers = 8\n")

		Register[checkedSection]()
		Register[checkedPointerSection]()
		require.NoError(t, Init())
		require.Equal(t, 8, Get[checkedSection]().Workers)
		require.Equal(t, 4, Get[*checkedPointerSection]().Workers, "the default tag passes the check")
	})
}

// TestInitReportsEveryRegisteredSectionItCannotLoad pins that one Init names
// the mistakes of every registered section, in the order of the sections'
// names, rather than those of whichever section it happened to load first.
func TestInitReportsEveryRegisteredSectionItCannotLoad(t *testing.T) {
	withFreshRegistry(t, "[sample_section]\nendpiont = file.example:8080\n\n[checked_section]\nworkers = 0\n")

	Register[sampleSection]()
	Register[checkedSection]()
	require.EqualError(t, Init(), `config: section "checked_section" of config.checkedSection: workers must be at least 1, got 0
config: section "sample_section" has no key "endpiont"`)
}

// TestCheckUnknownSectionsRefusesASectionNothingClaims pins the check a
// service runs right after Init: a section of the configuration file that is
// neither one of the framework's own nor registered fails it by name, every
// such section at once in sorted order, and so does a key the file keeps
// outside any section; the framework's sections and the registered ones
// pass. The check asks for Init to have loaded the configuration.
func TestCheckUnknownSectionsRefusesASectionNothingClaims(t *testing.T) {
	t.Run("misspelled_section", func(t *testing.T) {
		withFreshRegistry(t, "[reids]\naddr = 127.0.0.1:6379\n\n[server]\nport = 8097\n")

		require.NoError(t, Init())
		require.EqualError(t, CheckUnknownSections(), `config: section "reids" is neither the framework's nor registered`)
	})

	t.Run("every_unknown_section_in_order", func(t *testing.T) {
		withFreshRegistry(t, "[zone]\nname = a\n\n[reids]\naddr = 127.0.0.1:6379\n")

		require.NoError(t, Init())
		require.EqualError(t, CheckUnknownSections(),
			`config: section "reids" is neither the framework's nor registered; section "zone" is neither the framework's nor registered`)
	})

	t.Run("key_outside_any_section", func(t *testing.T) {
		withFreshRegistry(t, "stray = 1\n\n[server]\nport = 8097\n")

		require.NoError(t, Init())
		require.EqualError(t, CheckUnknownSections(), `config: section "default" is neither the framework's nor registered`)
	})

	t.Run("framework_and_registered_sections", func(t *testing.T) {
		withFreshRegistry(t, "[server]\nport = 8097\n\n[sample_section]\nretries = 7\n")

		Register[sampleSection]()
		require.NoError(t, Init())
		require.NoError(t, CheckUnknownSections())
	})

	t.Run("unregistered_project_section", func(t *testing.T) {
		withFreshRegistry(t, "[sample_section]\nretries = 7\n")

		require.NoError(t, Init())
		require.EqualError(t, CheckUnknownSections(), `config: section "sample_section" is neither the framework's nor registered`)
	})

	t.Run("before_init", func(t *testing.T) {
		withFreshRegistry(t, "")
		mu.Lock()
		cv = nil
		mu.Unlock()

		require.EqualError(t, CheckUnknownSections(), "config: CheckUnknownSections before Init has loaded the configuration")
	})
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
		cases := []struct {
			name     string
			register func()
			tag      string
		}{
			{
				name: "duration",
				register: func() {
					Register[struct {
						Timeout time.Duration `mapstructure:"timeout" default:"soon"`
					}]()
				},
				tag: "soon",
			},
			{
				name: "duration_pointer",
				register: func() {
					Register[struct {
						Wait *time.Duration `mapstructure:"wait" default:"soon"`
					}]()
				},
				tag: "soon",
			},
			{
				name: "bool",
				register: func() {
					Register[struct {
						Enabled bool `mapstructure:"enabled" default:"yes-please"`
					}]()
				},
				tag: "yes-please",
			},
			{
				name: "int",
				register: func() {
					Register[struct {
						Retries int `mapstructure:"retries" default:"three"`
					}]()
				},
				tag: "three",
			},
			{
				name: "uint_out_of_range",
				register: func() {
					Register[struct {
						Port uint16 `mapstructure:"port" default:"70000"`
					}]()
				},
				tag: "70000",
			},
			{
				name: "float",
				register: func() {
					Register[struct {
						Ratio float64 `mapstructure:"ratio" default:"half"`
					}]()
				},
				tag: "half",
			},
			{
				name: "pointer_in_nested_struct",
				register: func() {
					Register[struct {
						Nested struct {
							Enabled *bool `mapstructure:"enabled" default:"ture"`
						} `mapstructure:"nested"`
					}]()
				},
				tag: "ture",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				withFreshRegistry(t, "")

				tc.register()
				require.ErrorContains(t, Init(), `invalid default "`+tc.tag+`"`)
			})
		}
	})
}

// TestRegisterRefusesANonStructType pins that a registration of anything but
// a struct type, or a pointer to one, is a declaration error reported the
// way a taken section is: by Init, once, and as a panic after Init.
func TestRegisterRefusesANonStructType(t *testing.T) {
	withFreshRegistry(t, "")

	Register[string]()
	Register[*string]()
	Register[[]string]()
	Register[map[string]string]()
	Register[any]()

	err := Init()
	require.ErrorContains(t, err, "config: Register takes a struct type, or a pointer to one, not string")
	require.ErrorContains(t, err, "not []string")
	require.ErrorContains(t, err, "not map[string]string")
	require.ErrorContains(t, err, "not interface {}")
	require.NoError(t, Init(), "a refused registration is reported once")
	require.Panics(t, func() { Register[int]() })
	require.Empty(t, Get[string]())
	require.Nil(t, Get[any]())
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

// TestRegisterPanicsAfterInit pins that sections are registered before Init:
// a type registered once Init has loaded the configuration, which no load
// would ever read, panics naming the type and where a registration belongs,
// while registering again a type Init loaded is no conflict.
func TestRegisterPanicsAfterInit(t *testing.T) {
	withFreshRegistry(t, "")

	Register[sampleSection]()
	require.NoError(t, Init())

	require.PanicsWithError(t,
		"config: Register[config.checkedSection] after Init: register a section from an init function, before the configuration is loaded",
		func() { Register[checkedSection]() })
	require.NotPanics(t, func() { Register[sampleSection]() })
	require.NotPanics(t, func() { Register[*sampleSection]() })
}

// TestRegisterTakesAPointerType pins that registering a pointer to a struct
// type registers the struct type: the section loads the same, and Get returns
// it for either form.
func TestRegisterTakesAPointerType(t *testing.T) {
	withFreshRegistry(t, "[sample_section]\nretries = 7\n")

	Register[*sampleSection]()
	require.NoError(t, Init())

	require.Equal(t, 7, Get[*sampleSection]().Retries)
	require.Equal(t, 7, Get[sampleSection]().Retries)
	require.Equal(t, "127.0.0.1:8080", Get[sampleSection]().Endpoint)
}

// loggerHTTP is a section whose name and key together spell
// LOGGER_HTTP_BODY_ENABLED, the variable the framework's own
// logger.http_body.enabled key reads.
type loggerHTTP struct {
	BodyEnabled bool `mapstructure:"body_enabled"`
}

// TestRegisterRefusesAKeySharingAVariable pins the refusal: one environment
// variable setting two keys is a deployment that cannot say which it meant,
// and the value would land in both. The clash is in the variable, not in the
// section name — "logger_http" is free, and its key still collides.
func TestRegisterRefusesAKeySharingAVariable(t *testing.T) {
	withFreshRegistry(t, "")

	Register[loggerHTTP]()
	require.EqualError(t, Init(),
		`config: config.loggerHTTP registers key "logger_http.body_enabled", which reads the same environment variable LOGGER_HTTP_BODY_ENABLED as "logger.http_body.enabled"`)
	require.NoError(t, Init(), "a refused registration is reported once")
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
