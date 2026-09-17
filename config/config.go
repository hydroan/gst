// Package config loads the application configuration: the framework's own
// sections, held by App, and the sections a project adds with Register.
//
// # Resolution
//
// Every configuration key, the framework's own and those of a registered
// section alike, resolves in this priority, from highest to lowest:
//
//  1. Environment variables
//  2. The configuration file
//  3. Default values: the framework's own, and the "default" struct tags of
//     a registered section
//
// A key's environment variable is the key in upper case with its dots turned
// into underscores: server.port reads SERVER_PORT, logger.http_body.enabled
// reads LOGGER_HTTP_BODY_ENABLED, and the endpoint field of a registered
// Sample section reads SAMPLE_ENDPOINT. A key reads its variable whether or
// not the file mentions the key and whether or not it has a default. A
// variable set to an empty string, false or 0 takes effect like any other
// value: an empty string clears a string and zeroes a number, a bool or a
// list. A variable whose name maps to no key reads nothing and is not
// reported — one named like a section rather than a key, such as SERVER,
// included: a process shares its environment with everything else running
// in it, such as the variables a container platform injects.
//
// A registered type's section is its name in snake case: a SampleConfig type
// reads the [sample_config] section and the SAMPLE_CONFIG_ variables.
//
// # Failures
//
// Init fails, and the process with it, on configuration it cannot honor:
//
//   - A variable whose value does not decode into its key's type. The error
//     names the variable, its value and the key, such as a SERVER_PORT of
//     "tcp://10.0.0.1:8080" injected by a Kubernetes service link. A
//     duration or a map has no empty form, so an empty value fails too.
//   - A registered section named like a section of the framework's own
//     configuration, such as a type named Mysql.
//   - Two types registering the same section. Registering the same type again
//     is no conflict.
//   - A malformed "default" struct tag in a registered section.
//
// Init checks the registrations made before it and reports each refused one
// once; a registration made after Init that cannot be honored panics.
package config

import (
	"encoding"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/creasty/defaults"
	"github.com/spf13/viper"
	"github.com/stoewer/go-strcase"
	"go.uber.org/zap"
)

// Config is the framework's own configuration: one embedded struct per
// section, named by its mapstructure tag.
type Config struct {
	AppInfo       `json:"app" mapstructure:"app" ini:"app" yaml:"app"`
	Server        `json:"server" mapstructure:"server" ini:"server" yaml:"server"`
	Cache         `json:"cache" mapstructure:"cache" ini:"cache" yaml:"cache"`
	Middleware    `json:"middleware" mapstructure:"middleware" ini:"middleware" yaml:"middleware"`
	Auth          `json:"auth" mapstructure:"auth" ini:"auth" yaml:"auth"`
	Database      `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	Sqlite        `json:"sqlite" mapstructure:"sqlite" ini:"sqlite" yaml:"sqlite"`
	Postgres      `json:"postgres" mapstructure:"postgres" ini:"postgres" yaml:"postgres"`
	MySQL         `json:"mysql" mapstructure:"mysql" ini:"mysql" yaml:"mysql"`
	Clickhouse    `json:"clickhouse" mapstructure:"clickhouse" ini:"clickhouse" yaml:"clickhouse"`
	Redis         `json:"redis" mapstructure:"redis" ini:"redis" yaml:"redis"`
	OTEL          `json:"otel" mapstructure:"otel" ini:"otel" yaml:"otel"`
	Elasticsearch `json:"elasticsearch" mapstructure:"elasticsearch" ini:"elasticsearch" yaml:"elasticsearch"`
	Mongo         `json:"mongo" mapstructure:"mongo" ini:"mongo" yaml:"mongo"`
	Kafka         `json:"kafka" mapstructure:"kafka" ini:"kafka" yaml:"kafka"`
	Minio         `json:"minio" mapstructure:"minio" ini:"minio" yaml:"minio"`
	Logger        `json:"logger" mapstructure:"logger" ini:"logger" yaml:"logger"`
	Ldap          `json:"ldap" mapstructure:"ldap" ini:"ldap" yaml:"ldap"`
	Influxdb      `json:"influxdb" mapstructure:"influxdb" ini:"influxdb" yaml:"influxdb"`
	Mqtt          `json:"mqtt" mapstructure:"mqtt" ini:"mqtt" yaml:"mqtt"`
	Nats          `json:"nats" mapstructure:"nats" ini:"nats" yaml:"nats"`
	Etcd          `json:"etcd" mapstructure:"etcd" ini:"etcd" yaml:"etcd"`
	Cassandra     `json:"cassandra" mapstructure:"cassandra" ini:"cassandra" yaml:"cassandra"`
	Scylla        `json:"scylla" mapstructure:"scylla" ini:"scylla" yaml:"scylla"`
	RethinkDB     `json:"rethinkdb" mapstructure:"rethinkdb" ini:"rethinkdb" yaml:"rethinkdb"`
	RocketMQ      `json:"rocketmq" mapstructure:"rocketmq" ini:"rocketmq" yaml:"rocketmq"`
	Debug         `json:"debug" mapstructure:"debug" ini:"debug" yaml:"debug"`
	Audit         `json:"audit" mapstructure:"audit" ini:"audit" yaml:"audit"`
	Logmgmt       `json:"logmgmt" mapstructure:"logmgmt" ini:"logmgmt" yaml:"logmgmt"`
}

// setDefault sets the default value of every framework key on v.
func (c *Config) setDefault(v *viper.Viper) {
	c.AppInfo.setDefault(v)
	c.Server.setDefault(v)
	c.Cache.setDefault(v)
	c.Middleware.setDefault(v)
	c.Auth.setDefault(v)
	c.Logger.setDefault(v)
	c.Database.setDefault(v)
	c.Sqlite.setDefault(v)
	c.Postgres.setDefault(v)
	c.MySQL.setDefault(v)
	c.Clickhouse.setDefault(v)
	c.Redis.setDefault(v)
	c.OTEL.setDefault(v)
	c.Elasticsearch.setDefault(v)
	c.Mongo.setDefault(v)
	c.Kafka.setDefault(v)
	c.Ldap.setDefault(v)
	c.Influxdb.setDefault(v)
	c.Minio.setDefault(v)
	c.Mqtt.setDefault(v)
	c.Nats.setDefault(v)
	c.Etcd.setDefault(v)
	c.Cassandra.setDefault(v)
	c.Scylla.setDefault(v)
	c.RethinkDB.setDefault(v)
	c.RocketMQ.setDefault(v)
	c.Debug.setDefault(v)
	c.Audit.setDefault(v)
	c.Logmgmt.setDefault(v)
}

// defaultConfigName is the base name of the configuration file Init looks for.
const defaultConfigName = "config"

var (
	// App holds the framework's own configuration Init loads.
	App = new(Config)

	// cv is the viper instance Init loads the configuration into and Save
	// writes out.
	cv *viper.Viper
	// configFile is the configuration file SetConfigFile names; when empty,
	// Init looks for one in the working directory.
	configFile = ""
	// tempdir is the directory Init creates for the process outside tests,
	// which Clean removes.
	tempdir string
)

// Init initializes the application configuration: App and every section
// registered so far, each key resolved from the environment, the file and the
// defaults in that order. It fails on the configuration it cannot honor; the
// package documentation lists the rules and the failures.
func Init() (err error) {
	// Create temp directory if not in test.
	if flag.Lookup("test.v") == nil {
		if tempdir, err = os.MkdirTemp("", "gst_"); err != nil {
			return errors.Wrap(err, "failed to create temp dir")
		}
	}

	if cv, err = newViper(); err != nil {
		return err
	}
	// Every key is bound to its variable, see bindEnv. Viper's automatic
	// lookup stays off: it reads a variable named like a section, such as
	// SERVER, as that section, and hides every key under it.
	cv.AllowEmptyEnv(true)
	cv.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set default values before unmarshaling
	App = new(Config)
	App.setDefault(cv)

	cv.AddConfigPath(".")

	if err = readConfigFile(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			// Only create config file if not in test.
			if flag.Lookup("test.v") == nil {
				if err = os.WriteFile(filepath.Join(tempdir, fmt.Sprintf("%s.%s", defaultConfigName, defaultConfigTypes()[0])), nil, 0o600); err != nil {
					return errors.Wrap(err, "failed to create config file")
				}
			}
		} else {
			return errors.Wrap(err, "failed to read config file")
		}
	}
	if err = bindEnv("", reflect.ValueOf(App).Elem()); err != nil {
		return err
	}
	if err = cv.Unmarshal(App); err != nil {
		return errors.Wrap(err, "failed to unmarshal config")
	}

	mu.Lock()
	defer mu.Unlock()
	if errRegister != nil {
		err, errRegister = errRegister, nil
		return err
	}
	for name, typ := range registeredTypes {
		if err = loadSection(name, typ); err != nil {
			return err
		}
	}
	initialized = true

	return nil
}

// SetConfigFile sets an explicit config file path.
// You should always call this function before `Init`.
func SetConfigFile(file string) {
	mu.Lock()
	defer mu.Unlock()
	configFile = file
}

// readConfigFile reads the file SetConfigFile named, or else the first
// config file of a default type in the working directory, in the order
// defaultConfigTypes lists them. Without one, it leaves the search to viper,
// which reports viper.ConfigFileNotFoundError when it finds none either.
func readConfigFile() error {
	if len(configFile) > 0 {
		cv.SetConfigFile(configFile)
		return cv.ReadInConfig()
	}

	for _, typ := range defaultConfigTypes() {
		filename := fmt.Sprintf("%s.%s", defaultConfigName, typ)
		info, err := os.Stat(filename)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return errors.Wrapf(err, "failed to inspect config file %s", filename)
		}
		if info.IsDir() {
			continue
		}
		cv.SetConfigFile(filename)
		return cv.ReadInConfig()
	}

	cv.SetConfigName(defaultConfigName)
	cv.SetConfigType(defaultConfigTypes()[0])
	return cv.ReadInConfig()
}

// defaultConfigTypes lists the config file types Init looks for, in order;
// the first is the type of the empty config file it writes when there is none.
func defaultConfigTypes() []string {
	return []string{"ini", "yaml", "yml", "json", "toml"}
}

// Save writes the configuration Init loaded, every key as it resolved, to out
// in the config file's format.
func Save(out io.Writer) error {
	return cv.WriteConfigTo(out)
}

// TempDir returns the directory Init created for the process, empty in tests
// and before Init.
func TempDir() string {
	return tempdir
}

// Clean removes the directory Init created for the process and logs the
// outcome.
func Clean() {
	if err := os.RemoveAll(tempdir); err != nil {
		zap.S().Errorw("failed to remove temp dir", "error", err, "dir", tempdir)
	} else {
		zap.S().Infow("successfully remove temp dir", "dir", tempdir)
	}
}

var (
	// mu guards configFile and the registration state below.
	mu sync.RWMutex
	// registeredTypes maps each registered section to its type.
	registeredTypes = make(map[string]reflect.Type)
	// registeredConfigs maps each loaded section to a pointer to its value.
	registeredConfigs = make(map[string]any)
	// errRegister collects the registrations made before Init that Init must
	// refuse; Init reports them.
	errRegister error
	// initialized reports whether Init has run, after which a registration
	// loads its section at once.
	initialized bool
)

// builtinSections names the sections of the framework's own configuration,
// read off the Config struct, which no registered section may take.
var builtinSections = sync.OnceValue(func() map[string]bool {
	sections := make(map[string]bool)
	for field := range reflect.TypeFor[Config]().Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("mapstructure"), ",")
		sections[name] = true
	}
	return sections
})

// Register registers a custom configuration into the config system.
// The type parameter T can be either struct type or pointer to struct type.
// If T is not a struct or pointer to struct, the registration will be skipped silently.
// The registered type will be used to create and initialize the configuration
// instance when loading configuration from file or environment variables.
//
// The type's section is its name in snake case: SampleConfig reads the
// [sample_config] section, and a field reads the environment variable of its
// key, SAMPLE_CONFIG_ENDPOINT, or SAMPLE_CONFIG_TLS_CERT_FILE for a nested
// one. The section resolves like every key, see the package documentation.
//
// The struct tag "default" can be used to set default values for fields.
// For time.Duration fields, you can use duration strings like "5s", "1m", etc.
//
// Register can be called before or after `Init`. If called before `Init`,
// the registration will be processed during initialization, where Init fails
// on one it cannot honor; a registration after Init that cannot be honored
// panics. The package documentation lists what cannot be honored.
//
// Example usage:
//
//	type SampleConfig struct {
//		Endpoint string `json:"endpoint" mapstructure:"endpoint" default:"127.0.0.1:8080"`
//		Token    string `json:"token" mapstructure:"token"`
//		Enabled  bool   `json:"enabled" mapstructure:"enabled"`
//	}
//
//	type NotifierConfig struct {
//		URL      string        `json:"url" mapstructure:"url" default:"http://127.0.0.1:9000"`
//		Username string        `json:"username" mapstructure:"username" default:"notifier"`
//		Password string        `json:"password" mapstructure:"password"`
//		Timeout  time.Duration `json:"timeout" mapstructure:"timeout" default:"5s"`
//		Enabled  bool          `json:"enabled" mapstructure:"enabled"`
//	}
//
//	// Register with struct type
//	config.Register[SampleConfig]()
//
//	// Register with pointer type
//	config.Register[*NotifierConfig]()
//
// After registration, you can access the config using Get:
//
//	notifierCfg := config.Get[NotifierConfig]()
//	// or with pointer
//	notifierPtr := config.Get[*NotifierConfig]()
func Register[T any]() {
	mu.Lock()
	defer mu.Unlock()

	var t T
	typ := reflect.TypeOf(t)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	// Skip if not a struct type
	if typ.Kind() != reflect.Struct {
		return
	}

	section := sectionName(typ)
	err := checkSection(section, typ)
	if err == nil {
		registeredTypes[section] = typ
		if initialized {
			err = loadSection(section, typ)
		}
	}
	switch {
	case err == nil:
	case initialized:
		panic(err)
	default:
		errRegister = errors.Join(errRegister, err)
	}
}

// Get returns the registered custom configuration.
// The type parameter T must match the registered type or be a pointer to it,
// otherwise a zero value or nil pointer will be returned.
//
// Example usage:
//
//	config.Register[SampleConfig]()
//
//	// Get by struct type - returns a copy
//	cfg1 := config.Get[SampleConfig]()
//
//	// Get by pointer type - returns pointer
//	cfg2 := config.Get[*SampleConfig]()
//
//	// Not registered - returns zero value
//	cfg3 := config.Get[OtherConfig]()
//
//	// Not registered - returns nil
//	cfg4 := config.Get[*OtherConfig]()
func Get[T any]() (t T) {
	mu.RLock()
	defer mu.RUnlock()

	typ := reflect.TypeOf(t)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return t
	}
	section := sectionName(typ)

	stored, exists := registeredConfigs[section]
	if !exists {
		zap.S().Warnw("config not found", "name", section)
		return t
	}

	storedVal := reflect.ValueOf(stored)
	storedTyp := storedVal.Elem().Type()
	destTyp := reflect.TypeOf(t)

	if storedTyp == destTyp {
		return storedVal.Elem().Interface().(T) //nolint:errcheck
	}
	if destTyp.Kind() == reflect.Pointer {
		if storedTyp == destTyp.Elem() {
			return storedVal.Interface().(T) //nolint:errcheck
		}
	}

	zap.S().Warnw("config type mismatch", "name", section, "stored", storedTyp.Name(), "dest", destTyp.Name())
	return t
}

// sectionName returns the section a registered type reads: its name in snake
// case.
func sectionName(typ reflect.Type) string {
	return strings.ToLower(strcase.SnakeCase(typ.Name()))
}

// checkSection refuses a section no registered type may take: a section of
// the framework's own configuration, or the section of another registered
// type. Registering the same type again is no conflict.
func checkSection(name string, typ reflect.Type) error {
	if builtinSections()[name] {
		return errors.Newf("config: %s registers section %q, which the framework's own configuration uses", typ, name)
	}
	if registered, ok := registeredTypes[name]; ok && registered != typ {
		return errors.Newf("config: %s and %s both register section %q", registered, typ, name)
	}
	return nil
}

// loadSection loads a registered section: its default tags make the section's
// default values, every field reads its environment variable, and the section
// resolves from the environment, the file and the defaults in that order.
func loadSection(name string, typ reflect.Type) error {
	defaultCfg := reflect.New(typ)
	if err := defaults.Set(defaultCfg.Interface()); err != nil {
		return errors.Wrapf(err, "config: section %q of %s: failed to apply the default tags", name, typ)
	}
	if err := checkDefaultTags(typ); err != nil {
		return errors.Wrapf(err, "config: section %q of %s", name, typ)
	}
	setDefaultDurationFields(typ, defaultCfg.Elem())
	walkConfigKeys(name, defaultCfg.Elem(), func(key string, field reflect.Value) {
		cv.SetDefault(key, field.Interface())
	})
	if err := bindEnv(name, defaultCfg.Elem()); err != nil {
		return err
	}

	// The section decodes through Unmarshal into a struct holding it rather
	// than through UnmarshalKey: UnmarshalKey reads the keys under a section
	// from the file and the defaults alone, missing their environment
	// variables.
	holder := reflect.New(reflect.StructOf([]reflect.StructField{{
		Name: "Section",
		Type: typ,
		Tag:  reflect.StructTag(`mapstructure:"` + name + `"`),
	}}))
	if err := cv.Unmarshal(holder.Interface()); err != nil {
		return errors.Wrapf(err, "config: section %q of %s: failed to decode", name, typ)
	}
	section := holder.Elem().Field(0)
	keepUnkeyedDefaults(section, defaultCfg.Elem())
	registeredConfigs[name] = section.Addr().Interface()
	return nil
}

// keepUnkeyedDefaults sets each field of section, a registered section as
// decoded, that no configuration key reads — tagged mapstructure:"-" — to its
// value in defaultValues, the section's default values: decoding leaves such a
// field zero, and its default tag is the only value it has. Nested structs,
// and pointers to them, are walked into.
func keepUnkeyedDefaults(section, defaultValues reflect.Value) {
	for i := range section.NumField() {
		sf := section.Type().Field(i)
		if !sf.IsExported() {
			continue
		}
		field, defaultValue := section.Field(i), defaultValues.Field(i)
		if name, _, _ := strings.Cut(sf.Tag.Get("mapstructure"), ","); name == "-" {
			field.Set(defaultValue)
			continue
		}
		if field.Kind() == reflect.Pointer {
			if field.IsNil() || defaultValue.IsNil() {
				continue
			}
			field, defaultValue = field.Elem(), defaultValue.Elem()
		}
		if field.Kind() == reflect.Struct && field.Type() != reflect.TypeFor[time.Time]() {
			keepUnkeyedDefaults(field, defaultValue)
		}
	}
}

// checkDefaultTags reports each "default" struct tag of typ, a registered
// section's struct type, that package defaults cannot set its field from —
// it leaves such a field zero without a word — walking into nested structs
// and pointers to them. A field tagged "-" is passed over, as package defaults
// passes over it, and a list, a map or a struct fails in defaults.Set already.
func checkDefaultTags(typ reflect.Type) error {
	var errs error
	for sf := range typ.Fields() {
		tag := sf.Tag.Get("default")
		if !sf.IsExported() || tag == "-" {
			continue
		}
		fieldTyp := sf.Type
		for fieldTyp.Kind() == reflect.Pointer {
			fieldTyp = fieldTyp.Elem()
		}
		if fieldTyp.Kind() == reflect.Struct && fieldTyp != reflect.TypeFor[time.Time]() {
			errs = errors.Join(errs, checkDefaultTags(fieldTyp))
			continue
		}
		if tag == "" {
			continue
		}
		if err := parseDefaultTag(tag, fieldTyp); err != nil {
			errs = errors.Join(errs, errors.Wrapf(err, "field %s: default tag %q", sf.Name, tag))
		}
	}
	return errs
}

// parseDefaultTag reports whether package defaults can set a value of typ
// from tag, trying what it tries in its order: the type's own text or JSON
// unmarshaling, then its kind. A kind it sets without parsing, such as a
// string, takes any tag.
func parseDefaultTag(tag string, typ reflect.Type) error {
	value := reflect.New(typ).Interface()
	if u, ok := value.(encoding.TextUnmarshaler); ok && u.UnmarshalText([]byte(tag)) == nil {
		return nil
	}
	if u, ok := value.(json.Unmarshaler); ok && u.UnmarshalJSON([]byte(tag)) == nil {
		return nil
	}
	var err error
	switch typ.Kind() {
	case reflect.Bool:
		_, err = strconv.ParseBool(tag)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		_, err = strconv.ParseInt(tag, 0, typ.Bits())
	case reflect.Int64:
		// A duration first, then an integer; a time.Duration reports why the
		// tag is no duration.
		_, durationErr := time.ParseDuration(tag)
		if durationErr == nil {
			break
		}
		if _, err = strconv.ParseInt(tag, 0, 64); err != nil && typ == reflect.TypeFor[time.Duration]() {
			err = durationErr
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		_, err = strconv.ParseUint(tag, 0, typ.Bits())
	case reflect.Float32, reflect.Float64:
		_, err = strconv.ParseFloat(tag, typ.Bits())
	}
	return err
}

// setDefaultDurationFields sets each zero time.Duration field of val, a value
// of struct type typ, from its "default" struct tag, walking into nested and
// embedded structs and pointers to them — a time.Time is a value, not one to
// walk into — and passing over a field tagged "-". A tag that does not parse
// leaves its field alone; checkDefaultTags reports it.
func setDefaultDurationFields(typ reflect.Type, val reflect.Value) {
	if typ.Kind() != reflect.Struct {
		return
	}
	for i := range typ.NumField() {
		fieldTyp := typ.Field(i)
		fieldVal := val.Field(i)
		defaultValue, tagged := fieldTyp.Tag.Lookup("default")
		if defaultValue == "-" {
			continue
		}

		// Handle embedded structs, an unexported one included: the fields it
		// promotes are the struct's own.
		if fieldTyp.Anonymous && fieldTyp.Type.Kind() == reflect.Struct {
			setDefaultDurationFields(fieldTyp.Type, fieldVal)
			continue
		}
		if !fieldTyp.IsExported() {
			// Reflection cannot set any other unexported field.
			continue
		}

		// Handle time.Duration field
		if fieldTyp.Type == reflect.TypeFor[time.Duration]() && tagged && fieldVal.Interface().(time.Duration) == 0 { //nolint:errcheck
			if duration, err := time.ParseDuration(defaultValue); err == nil {
				fieldVal.Set(reflect.ValueOf(duration))
			}
		}

		// Recursively process nested structs (if not embedded)
		if fieldTyp.Type.Kind() == reflect.Struct && fieldTyp.Type != reflect.TypeFor[time.Time]() {
			setDefaultDurationFields(fieldTyp.Type, fieldVal)
		}

		// Handle pointer to struct
		if fieldTyp.Type.Kind() == reflect.Pointer && fieldTyp.Type.Elem().Kind() == reflect.Struct && fieldTyp.Type.Elem() != reflect.TypeFor[time.Time]() {
			// If the pointer is nil, initialize it
			if fieldVal.IsNil() {
				fieldVal.Set(reflect.New(fieldTyp.Type.Elem()))
			}
			setDefaultDurationFields(fieldTyp.Type.Elem(), fieldVal.Elem())
		}
	}
}

// bindEnv binds every key of the configuration struct cfg under prefix to its
// environment variable, so a key no file or default mentions reads it too, and
// checks the variables that are set decode into their keys' types, so a value
// that cannot fails naming the variable that carries it.
func bindEnv(prefix string, cfg reflect.Value) error {
	var errs error
	walkConfigKeys(prefix, cfg, func(key string, field reflect.Value) {
		if err := cv.BindEnv(key); err != nil {
			errs = errors.Join(errs, errors.Wrapf(err, "config: failed to bind the environment variable of %s", key))
			return
		}
		name := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		value, ok := os.LookupEnv(name)
		if !ok {
			return
		}
		if err := decodeEnvValue(value, field.Type()); err != nil {
			errs = errors.Join(errs, errors.Newf("config: environment variable %s=%q is not a valid %s for %s: %v", name, value, field.Type(), key, err))
		}
	})
	return errs
}

// decodeEnvValue decodes an environment variable's value into typ the way
// Unmarshal decodes it, through a viper holding that value alone.
func decodeEnvValue(value string, typ reflect.Type) error {
	v := viper.New()
	v.Set("value", value)
	return v.UnmarshalKey("value", reflect.New(typ).Interface())
}

// walkConfigKeys calls visit with every key of the configuration struct cfg
// under prefix and the field holding its value, the keys viper decodes: a
// field is keyed by its mapstructure name, else its field name in lower case,
// a squashed field adds no key of its own — an unexported embedded struct
// squashed in counts, its fields are decoded all the same — and a nested
// struct, or a pointer to one, is walked into. A time.Time, or a pointer to
// one, is a single value.
func walkConfigKeys(prefix string, cfg reflect.Value, visit func(key string, field reflect.Value)) {
	for sf, field := range cfg.Fields() {
		name, options, _ := strings.Cut(sf.Tag.Get("mapstructure"), ",")
		squash := slices.Contains(strings.Split(options, ","), "squash")
		decoded := sf.IsExported() || squash && sf.Anonymous && sf.Type.Kind() == reflect.Struct
		if name == "-" || !decoded {
			continue
		}
		key := prefix
		if !squash {
			if name == "" {
				name = sf.Name
			}
			key = strings.TrimPrefix(prefix+"."+strings.ToLower(name), ".")
		}
		if field.Kind() == reflect.Pointer && field.Type().Elem().Kind() == reflect.Struct && field.Type().Elem() != reflect.TypeFor[time.Time]() {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			field = field.Elem()
		}
		if field.Kind() == reflect.Struct && field.Type() != reflect.TypeFor[time.Time]() {
			walkConfigKeys(key, field, visit)
			continue
		}
		visit(key, field)
	}
}
