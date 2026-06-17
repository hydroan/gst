package config

import "time"

const (
	INFLUXDB_HOST   = "INFLUXDB_HOST"   //nolint:staticcheck
	INFLUXDB_PORT   = "INFLUXDB_PORT"   //nolint:staticcheck
	INFLUXDB_TOKEN  = "INFLUXDB_TOKEN"  //nolint:staticcheck,gosec
	INFLUXDB_ORG    = "INFLUXDB_ORG"    //nolint:staticcheck
	INFLUXDB_BUCKET = "INFLUXDB_BUCKET" //nolint:staticcheck

	INFLUXDB_BATCH_SIZE         = "INFLUXDB_BATCH_SIZE"         //nolint:staticcheck
	INFLUXDB_FLUSH_INTERVAL     = "INFLUXDB_FLUSH_INTERVAL"     //nolint:staticcheck
	INFLUXDB_RETRY_INTERVAL     = "INFLUXDB_RETRY_INTERVAL"     //nolint:staticcheck
	INFLUXDB_MAX_RETRIES        = "INFLUXDB_MAX_RETRIES"        //nolint:staticcheck
	INFLUXDB_RETRY_BUFFER_LIMIT = "INFLUXDB_RETRY_BUFFER_LIMIT" //nolint:staticcheck
	INFLUXDB_MAX_RETRY_INTERVAL = "INFLUXDB_MAX_RETRY_INTERVAL" //nolint:staticcheck
	INFLUXDB_MAX_RETRY_TIME     = "INFLUXDB_MAX_RETRY_TIME"     //nolint:staticcheck
	INFLUXDB_EXPONENTIAL_BASE   = "INFLUXDB_EXPONENTIAL_BASE"   //nolint:staticcheck
	INFLUXDB_PRECISION          = "INFLUXDB_PRECISION"          //nolint:staticcheck
	INFLUXDB_USE_GZIP           = "INFLUXDB_USE_GZIP"           //nolint:staticcheck

	INFLUXDB_ENABLE_TLS           = "INFLUXDB_ENABLE_TLS"           //nolint:staticcheck
	INFLUXDB_CERT_FILE            = "INFLUXDB_CERT_FILE"            //nolint:staticcheck
	INFLUXDB_KEY_FILE             = "INFLUXDB_KEY_FILE"             //nolint:staticcheck
	INFLUXDB_CA_FILE              = "INFLUXDB_CA_FILE"              //nolint:staticcheck
	INFLUXDB_INSECURE_SKIP_VERIFY = "INFLUXDB_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	//nolint:staticcheck
	INFLUXDB_DEFAULT_TAGS = "INFLUXDB_DEFAULT_TAGS" // format：key1=value1,key2=value2
	INFLUXDB_APP_NAME     = "INFLUXDB_APP_NAME"     //nolint:staticcheck

	INFLUXDB_ENABLE = "INFLUXDB_ENABLE" //nolint:staticcheck
)

type Influxdb struct {
	Host   string `json:"host" mapstructure:"host" ini:"host" yaml:"host"`
	Port   uint   `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Token  string `json:"token" mapstructure:"token" ini:"token" yaml:"token"`
	Org    string `json:"org" mapstructure:"org" ini:"org" yaml:"org"`
	Bucket string `json:"bucket" mapstructure:"bucket" ini:"bucket" yaml:"bucket"`

	// Write options
	BatchSize        uint          `json:"batch_size" mapstructure:"batch_size" ini:"batch_size" yaml:"batch_size"`
	FlushInterval    time.Duration `json:"flush_interval" mapstructure:"flush_interval" ini:"flush_interval" yaml:"flush_interval"`
	RetryInterval    time.Duration `json:"retry_interval" mapstructure:"retry_interval" ini:"retry_interval" yaml:"retry_interval"`
	MaxRetries       uint          `json:"max_retries" mapstructure:"max_retries" ini:"max_retries" yaml:"max_retries"`
	RetryBufferLimit uint          `json:"retry_buffer_limit" mapstructure:"retry_buffer_limit" ini:"retry_buffer_limit" yaml:"retry_buffer_limit"`
	MaxRetryInterval time.Duration `json:"max_retry_interval" mapstructure:"max_retry_interval" ini:"max_retry_interval" yaml:"max_retry_interval"`
	MaxRetryTime     time.Duration `json:"max_retry_time" mapstructure:"max_retry_time" ini:"max_retry_time" yaml:"max_retry_time"`
	ExponentialBase  uint          `json:"exponential_base" mapstructure:"exponential_base" ini:"exponential_base" yaml:"exponential_base"`
	Precision        time.Duration `json:"precision" mapstructure:"precision" ini:"precision" yaml:"precision"`
	UseGZip          bool          `json:"use_gzip" mapstructure:"use_gzip" ini:"use_gzip" yaml:"use_gzip"`

	// TLS configuration
	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	// Advanced options
	DefaultTags map[string]string `json:"default_tags" mapstructure:"default_tags" ini:"default_tags" yaml:"default_tags"`
	AppName     string            `json:"app_name" mapstructure:"app_name" ini:"app_name" yaml:"app_name"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Influxdb) setDefault() {
	cv.SetDefault("influxdb.host", "127.0.0.1")
	cv.SetDefault("influxdb.port", 8086)
	cv.SetDefault("influxdb.token", "")
	cv.SetDefault("influxdb.org", "")
	cv.SetDefault("influxdb.bucket", "")

	cv.SetDefault("influxdb.batch_size", 0)
	cv.SetDefault("influxdb.flush_interval", 0)
	cv.SetDefault("influxdb.retry_interval", 0)
	cv.SetDefault("influxdb.max_retries", 0)
	cv.SetDefault("influxdb.retry_buffer_limit", 0)
	cv.SetDefault("influxdb.max_retry_interval", 0)
	cv.SetDefault("influxdb.max_retry_time", 0)
	cv.SetDefault("influxdb.exponential_base", 0)
	cv.SetDefault("influxdb.precision", 0)
	cv.SetDefault("influxdb.use_gzip", false)

	cv.SetDefault("influxdb.enable_tls", false)
	cv.SetDefault("influxdb.cert_file", "")
	cv.SetDefault("influxdb.key_file", 0)
	cv.SetDefault("influxdb.ca_file", "")
	cv.SetDefault("influxdb.insecure_skip_verify", false)

	cv.SetDefault("influxdb.default_tags", nil)
	cv.SetDefault("influxdb.app_name", "")

	cv.SetDefault("influxdb.enable", false)
}
