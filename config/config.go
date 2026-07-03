package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/creasty/defaults"
	"github.com/go-viper/encoding/ini"
	"github.com/spf13/viper"
	"github.com/stoewer/go-strcase"
	"go.uber.org/zap"
)

const (
	noneExpireToken   = `fake_token`
	noneExpireUser    = "admin"
	noneExpirePass    = "admin"
	baseAuthUsername  = "admin"
	baseAuthPassword  = "admin"
	defaultConfigName = "config"
)

var (
	App = new(Config)

	configFile = ""

	registeredConfigs = make(map[string]any)
	registeredTypes   = make(map[string]reflect.Type)

	inited  bool
	tempdir string
	mu      sync.RWMutex
	cv      *viper.Viper
)

type Config struct {
	AppInfo       `json:"app" mapstructure:"app" ini:"app" yaml:"app"`
	Server        `json:"server" mapstructure:"server" ini:"server" yaml:"server"`
	Middleware    `json:"middleware" mapstructure:"middleware" ini:"middleware" yaml:"middleware"`
	Grpc          `json:"grpc" mapstructure:"grpc" ini:"grpc" yaml:"grpc"`
	Auth          `json:"auth" mapstructure:"auth" ini:"auth" yaml:"auth"`
	Database      `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	Cache         `json:"cache" mapstructure:"cache" ini:"cache" yaml:"cache"`
	Sqlite        `json:"sqlite" mapstructure:"sqlite" ini:"sqlite" yaml:"sqlite"`
	Postgres      `json:"postgres" mapstructure:"postgres" ini:"postgres" yaml:"postgres"`
	MySQL         `json:"mysql" mapstructure:"mysql" ini:"mysql" yaml:"mysql"`
	SQLServer     `json:"sqlserver" mapstructure:"sqlserver" ini:"sqlserver" yaml:"sqlserver"`
	Clickhouse    `json:"clickhouse" mapstructure:"clickhouse" ini:"clickhouse" yaml:"clickhouse"`
	Redis         `json:"redis" mapstructure:"redis" ini:"redis" yaml:"redis"`
	OTEL          `json:"otel" mapstructure:"otel" ini:"otel" yaml:"otel"`
	Elasticsearch `json:"elasticsearch" mapstructure:"elasticsearch" ini:"elasticsearch" yaml:"elasticsearch"`
	Mongo         `json:"mongo" mapstructure:"mongo" ini:"mongo" yaml:"mongo"`
	Kafka         `json:"kafka" mapstructure:"kafka" ini:"kafka" yaml:"kafka"`
	Minio         `json:"minio" mapstructure:"minio" ini:"minio" yaml:"minio"`
	S3            `json:"s3" mapstructure:"s3" ini:"s3" yaml:"s3"`
	Logger        `json:"logger" mapstructure:"logger" ini:"logger" yaml:"logger"`
	Ldap          `json:"ldap" mapstructure:"ldap" ini:"ldap" yaml:"ldap"`
	Influxdb      `json:"influxdb" mapstructure:"influxdb" ini:"influxdb" yaml:"influxdb"`
	Mqtt          `json:"mqtt" mapstructure:"mqtt" ini:"mqtt" yaml:"mqtt"`
	Nats          `json:"nats" mapstructure:"nats" ini:"nats" yaml:"nats"`
	Etcd          `json:"etcd" mapstructure:"etcd" ini:"etcd" yaml:"etcd"`
	Cassandra     `json:"cassandra" mapstructure:"cassandra" ini:"cassandra" yaml:"cassandra"`
	Scylla        `json:"scylla" mapstructure:"scylla" ini:"scylla" yaml:"scylla"`
	Memcached     `json:"memcached" mapstructure:"memcached" ini:"memcached" yaml:"memcached"`
	RethinkDB     `json:"rethinkdb" mapstructure:"rethinkdb" ini:"rethinkdb" yaml:"rethinkdb"`
	RocketMQ      `json:"rocketmq" mapstructure:"rocketmq" ini:"rocketmq" yaml:"rocketmq"`
	Feishu        `json:"feishu" mapstructure:"feishu" ini:"feishu" yaml:"feishu"`
	Debug         `json:"debug" mapstructure:"debug" ini:"debug" yaml:"debug"`
	Audit         `json:"audit" mapstructure:"audit" ini:"audit" yaml:"audit"`
}

// setDefault will set config default value
func (c *Config) setDefault() {
	c.AppInfo.setDefault()
	c.Server.setDefault()
	c.Middleware.setDefault()
	c.Grpc.setDefault()
	c.Auth.setDefault()
	c.Logger.setDefault()
	c.Database.setDefault()
	c.Cache.setDefault()
	c.Sqlite.setDefault()
	c.Postgres.setDefault()
	c.MySQL.setDefault()
	c.Clickhouse.setDefault()
	c.SQLServer.setDefault()
	c.Redis.setDefault()
	c.OTEL.setDefault()
	c.Elasticsearch.setDefault()
	c.Mongo.setDefault()
	c.Kafka.setDefault()
	c.Ldap.setDefault()
	c.Influxdb.setDefault()
	c.Minio.setDefault()
	c.S3.setDefault()
	c.Mqtt.setDefault()
	c.Nats.setDefault()
	c.Etcd.setDefault()
	c.Cassandra.setDefault()
	c.Scylla.setDefault()
	c.Memcached.setDefault()
	c.RethinkDB.setDefault()
	c.RocketMQ.setDefault()
	c.Feishu.setDefault()
	c.Debug.setDefault()
	c.Audit.setDefault()
}

// Init initializes the application configuration
//
// Configuration priority (from highest to lowest):
// 1. Environment variables
// 2. Configuration file
// 3. Default values
func Init() (err error) {
	// Create temp directory if not in test.
	if flag.Lookup("test.v") == nil {
		if tempdir, err = os.MkdirTemp("", "gst_"); err != nil {
			return errors.Wrap(err, "failed to create temp dir")
		}
		// logger not initialized using fmt.Println instead.
		fmt.Fprintf(os.Stdout, "create temp dir: %s\n", tempdir)
	}

	// Breaking change:
	// https://github.com/spf13/viper/blob/master/UPGRADE.md#breaking-hcl-java-properties-ini-removed-from-core
	codecRegistry := viper.NewCodecRegistry()
	if err = codecRegistry.RegisterCodec("ini", ini.Codec{}); err != nil {
		return err
	}
	cv = viper.NewWithOptions(viper.WithCodecRegistry(codecRegistry))
	cv.AutomaticEnv()
	cv.AllowEmptyEnv(true)
	cv.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set default values before unmarshaling
	App = new(Config)
	App.setDefault()

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
	if err = cv.Unmarshal(App); err != nil {
		return errors.Wrap(err, "failed to unmarshal config")
	}

	for name, typ := range registeredTypes {
		registerType(name, typ)
	}
	inited = true

	return nil
}

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

func defaultConfigTypes() []string {
	return []string{"ini", "yaml", "yml", "json", "toml"}
}

func Clean() {
	if err := os.RemoveAll(tempdir); err != nil {
		zap.S().Errorw("failed to remove temp dir", "error", err, "dir", tempdir)
	} else {
		zap.S().Infow("successfully remove temp dir", "dir", tempdir)
	}
}

func Tempdir() string {
	return tempdir
}

// Register registers a custom configuration into config system.
// The type parameter T can be either struct type or pointer to struct type.
// If T is not a struct or pointer to struct, the registration will be skipped silently.
// The registered type will be used to create and initialize the configuration
// instance when loading configuration from file or environment variables.
//
// Configuration values are loaded in the following priority order (from highest to lowest):
// 1. Environment variables (format: SECTION_FIELD, e.g., NATS_USERNAME)
// 2. Configuration file values
// 3. Default values from struct tags
//
// The struct tag "default" can be used to set default values for fields.
// For time.Duration fields, you can use duration strings like "5s", "1m", etc.
//
// Register can be called before or after `Init`. If called before `Init`,
// the registration will be processed during initialization.
//
// Example usage:
//
//	type WechatConfig struct {
//		AppID     string `json:"app_id" mapstructure:"app_id" default:"myappid"`
//		AppSecret string `json:"app_secret" mapstructure:"app_secret" default:"myappsecret"`
//		Enabled bool   `json:"enabled" mapstructure:"enabled"`
//	}
//
//	type NatsConfig struct {
//		URL      string        `json:"url" mapstructure:"url" default:"nats://127.0.0.1:4222"`
//		Username string        `json:"username" mapstructure:"username" default:"nats"`
//		Password string        `json:"password" mapstructure:"password" default:"nats"`
//		Timeout  time.Duration `json:"timeout" mapstructure:"timeout" default:"5s"`
//		Enabled bool          `json:"enabled" mapstructure:"enabled"`
//	}
//
//	// Register with struct type
//	config.Register[WechatConfig]()
//
//	// Register with pointer type
//	config.Register[*NatsConfig]()
//
// After registration, you can access the config using Get:
//
//	natsCfg := config.Get[NatsConfig]()
//	// or with pointer
//	natsPtr := config.Get[*NatsConfig]()
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

	// Determine the configuration name
	cfgName := buildConfigName(typ)

	if inited {
		registerType(cfgName, typ)
	} else {
		registeredTypes[cfgName] = typ
	}
}

func registerType(name string, typ reflect.Type) {
	name = strings.ToLower(name)

	// Set default value from struct tag "default".
	cfg := reflect.New(typ).Interface()
	if err := defaults.Set(cfg); err != nil {
		zap.S().Warnw("failed to set default value", "name", name, "type", typ, "error", err)
	}
	// NOTE: package "defaults" not support set default value for time.Duration, so we should set it manually.
	setDefaultDurationFields(typ, reflect.ValueOf(cfg).Elem())

	// Set config value from config file.
	if err := cv.UnmarshalKey(name, cfg); err != nil {
		zap.S().Warnw("failed to unmarshal config", "name", name, "type", typ, "error", err)
	}

	// Set config value from environment variables.
	envCfg := reflect.New(typ).Interface()
	envPrefix := strings.ToUpper(name) + "_"
	v := reflect.ValueOf(envCfg).Elem()
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		mapstructureTag := field.Tag.Get("mapstructure")
		if len(mapstructureTag) == 0 {
			continue
		}
		envKey := envPrefix + strings.ToUpper(mapstructureTag)
		if envVal, exists := os.LookupEnv(envKey); exists {
			fieldVal := v.Field(i)
			switch fieldVal.Kind() {
			case reflect.String:
				fieldVal.SetString(envVal)
			case reflect.Bool:
				boolVal, err := strconv.ParseBool(envVal)
				if err == nil {
					fieldVal.SetBool(boolVal)
				}
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if field.Type == reflect.TypeFor[time.Duration]() {
					// handle time.Duration
					if duration, err := time.ParseDuration(envVal); err == nil {
						fieldVal.SetInt(int64(duration))
					}
				} else {
					if intVal, err := strconv.ParseInt(envVal, 10, 64); err == nil {
						fieldVal.SetInt(intVal)
					}
				}
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				if uintVal, err := strconv.ParseUint(envVal, 10, 64); err == nil {
					fieldVal.SetUint(uintVal)
				}
			case reflect.Float32, reflect.Float64:
				if floatVal, err := strconv.ParseFloat(envVal, 64); err == nil {
					fieldVal.SetFloat(floatVal)
				}

			}
		}
	}
	mergeNonZeroFields(reflect.ValueOf(cfg).Elem(), v)

	registeredConfigs[name] = cfg
}

func setDefaultDurationFields(typ reflect.Type, val reflect.Value) {
	if typ.Kind() != reflect.Struct {
		return
	}
	for i := range typ.NumField() {
		fieldTyp := typ.Field(i)
		fieldVal := val.Field(i)

		// Handle embedded structs
		if fieldTyp.Anonymous && fieldTyp.Type.Kind() == reflect.Struct {
			setDefaultDurationFields(fieldTyp.Type, fieldVal)
			continue
		}

		// Handle time.Duration field
		if fieldTyp.Type == reflect.TypeFor[time.Duration]() {
			// Check if the field has a default tag and its current value is zero
			if defaultValue, ok := fieldTyp.Tag.Lookup("default"); ok && fieldVal.Interface().(time.Duration) == 0 { //nolint:errcheck
				// Parse the duration string
				if duration, err := time.ParseDuration(defaultValue); err == nil {
					fieldVal.Set(reflect.ValueOf(duration))
				} else {
					zap.S().Warnw("failed to parse duration default value",
						"field", fieldTyp.Name,
						"default", defaultValue,
						"error", err)
				}
			}
		}

		// Recursively process nested structs (if not embedded)
		if fieldTyp.Type.Kind() == reflect.Struct && !fieldTyp.Anonymous {
			setDefaultDurationFields(fieldTyp.Type, fieldVal)
		}

		// Handle pointer to struct
		if fieldTyp.Type.Kind() == reflect.Pointer && fieldTyp.Type.Elem().Kind() == reflect.Struct {
			// If the pointer is nil, initialize it
			if fieldVal.IsNil() {
				fieldVal.Set(reflect.New(fieldTyp.Type.Elem()))
			}
			setDefaultDurationFields(fieldTyp.Type.Elem(), fieldVal.Elem())
		}
	}
}

func mergeNonZeroFields(dst, src reflect.Value) {
	for i := range src.NumField() {
		srcField := src.Field(i)
		if !isZeroValue(srcField) {
			dst.Field(i).Set(srcField)
		}
	}
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.String:
		return v.String() == ""
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}

// Get returns the registered custom configuration.
// The type parameter T must match the registered type or be a pointer to it,
// otherwise a zero value or nil pointer will be returned.
//
// Example usage:
//
//	config.Register[WechatConfig]()
//
//	// Get by pointer type - returns pointer
//	cfg3 := config.Get[*WechatConfig]()
//
//	// Type mismatch - returns zero value
//	cfg4 := config.Get[OtherConfig]()
//
//	// Type mismatch - returns nil
//	cfg5 := config.Get[*OtherConfig]()
func Get[T any]() (t T) {
	mu.RLock()
	defer mu.RUnlock()

	// Determine the configuration name
	var temp T
	typ := reflect.TypeOf(temp)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return t
	}
	cfgName := buildConfigName(typ)

	config, exists := registeredConfigs[cfgName]
	if !exists {
		zap.S().Warnw("config not found", "name", cfgName)
		return t
	}

	storedVal := reflect.ValueOf(config)
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

	zap.S().Warnw("config type mismatch", "name", cfgName, "stored", storedTyp.Name(), "dest", destTyp.Name())
	return t
}

// SetConfigFile sets an explicit config file path.
// You should always call this function before `Init`.
func SetConfigFile(file string) {
	mu.Lock()
	defer mu.Unlock()
	configFile = file
}

// Save config instance to destination io.Writer
func Save(out io.Writer) error {
	return cv.WriteConfigTo(out)
}

func buildConfigName(typ reflect.Type) string {
	return strings.ToLower(strcase.SnakeCase(typ.Name()))
}
