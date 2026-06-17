package config

import "time"

const (
	ELASTICSEARCH_ADDRS                   = "ELASTICSEARCH_ADDRS"                   //nolint:staticcheck
	ELASTICSEARCH_USERNAME                = "ELASTICSEARCH_USERNAME"                //nolint:staticcheck
	ELASTICSEARCH_PASSWORD                = "ELASTICSEARCH_PASSWORD"                //nolint:staticcheck
	ELASTICSEARCH_CLOUD_ID                = "ELASTICSEARCH_CLOUD_ID"                //nolint:staticcheck
	ELASTICSEARCH_API_KEY                 = "ELASTICSEARCH_API_KEY"                 //nolint:staticcheck
	ELASTICSEARCH_MAX_RETRIES             = "ELASTICSEARCH_MAX_RETRIES"             //nolint:staticcheck
	ELASTICSEARCH_RETRY_ON_STATUS         = "ELASTICSEARCH_RETRY_ON_STATUS"         //nolint:staticcheck
	ELASTICSEARCH_DISABLE_RETRIES         = "ELASTICSEARCH_DISABLE_RETRIES"         //nolint:staticcheck
	ELASTICSEARCH_RETRY_BACKOFF           = "ELASTICSEARCH_RETRY_BACKOFF"           //nolint:staticcheck
	ELASTICSEARCH_RETRY_BACKOFF_MIN       = "ELASTICSEARCH_RETRY_BACKOFF_MIN"       //nolint:staticcheck
	ELASTICSEARCH_RETRY_BACKOFF_MAX       = "ELASTICSEARCH_RETRY_BACKOFF_MAX"       //nolint:staticcheck
	ELASTICSEARCH_COMPRESS                = "ELASTICSEARCH_COMPRESS"                //nolint:staticcheck
	ELASTICSEARCH_DISCOVERY_INTERVAL      = "ELASTICSEARCH_DISCOVERY_INTERVAL"      //nolint:staticcheck
	ELASTICSEARCH_ENABLE_METRICS          = "ELASTICSEARCH_ENABLE_METRICS"          //nolint:staticcheck
	ELASTICSEARCH_ENABLE_DEBUG_LOGGER     = "ELASTICSEARCH_ENABLE_DEBUG_LOGGER"     //nolint:staticcheck
	ELASTICSEARCH_CONNECTION_POOL_SIZE    = "ELASTICSEARCH_CONNECTION_POOL_SIZE"    //nolint:staticcheck
	ELASTICSEARCH_RESPONSE_HEADER_TIMEOUT = "ELASTICSEARCH_RESPONSE_HEADER_TIMEOUT" //nolint:staticcheck
	ELASTICSEARCH_REQUEST_TIMEOUT         = "ELASTICSEARCH_REQUEST_TIMEOUT"         //nolint:staticcheck
	ELASTICSEARCH_DIAL_TIMEOUT            = "ELASTICSEARCH_DIAL_TIMEOUT"            //nolint:staticcheck
	ELASTICSEARCH_KEEP_ALIVE_INTERVAL     = "ELASTICSEARCH_KEEP_ALIVE_INTERVAL"     //nolint:staticcheck

	ELASTICSEARCH_ENABLE_TLS           = "ELASTICSEARCH_ENABLE_TLS"           //nolint:staticcheck
	ELASTICSEARCH_CERT_FILE            = "ELASTICSEARCH_CERT_FILE"            //nolint:staticcheck
	ELASTICSEARCH_KEY_FILE             = "ELASTICSEARCH_KEY_FILE"             //nolint:staticcheck
	ELASTICSEARCH_CA_FILE              = "ELASTICSEARCH_CA_FILE"              //nolint:staticcheck
	ELASTICSEARCH_INSECURE_SKIP_VERIFY = "ELASTICSEARCH_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	ELASTICSEARCH_ENABLE = "ELASTICSEARCH_ENABLE" //nolint:staticcheck
)

type Elasticsearch struct {
	Addrs                 []string      `json:"addrs" mapstructure:"addrs" ini:"addrs" yaml:"addrs"`
	Username              string        `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password              string        `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	CloudID               string        `json:"cloud_id" mapstructure:"cloud_id" ini:"cloud_id" yaml:"cloud_id"`
	APIKey                string        `json:"api_key" mapstructure:"api_key" ini:"api_key" yaml:"api_key"`
	MaxRetries            int           `json:"max_retries" mapstructure:"max_retries" ini:"max_retries" yaml:"max_retries"`
	RetryOnStatus         []int         `json:"retry_on_status" mapstructure:"retry_on_status" ini:"retry_on_status" yaml:"retry_on_status"`
	DisableRetries        bool          `json:"disable_retries" mapstructure:"disable_retries" ini:"disable_retries" yaml:"disable_retries"`
	RetryBackoff          bool          `json:"retry_backoff" mapstructure:"retry_backoff" ini:"retry_backoff" yaml:"retry_backoff"`
	RetryBackoffMin       time.Duration `json:"retry_backoff_min" mapstructure:"retry_backoff_min" ini:"retry_backoff_min" yaml:"retry_backoff_min"`
	RetryBackoffMax       time.Duration `json:"retry_backoff_max" mapstructure:"retry_backoff_max" ini:"retry_backoff_max" yaml:"retry_backoff_max"`
	Compress              bool          `json:"compress" mapstructure:"compress" ini:"compress" yaml:"compress"`
	DiscoveryInterval     time.Duration `json:"discovery_interval" mapstructure:"discovery_interval" ini:"discovery_interval" yaml:"discovery_interval"`
	EnableMetrics         bool          `json:"enable_metrics" mapstructure:"enable_metrics" ini:"enable_metrics" yaml:"enable_metrics"`
	EnableDebugLogger     bool          `json:"enable_debug_logger" mapstructure:"enable_debug_logger" ini:"enable_debug_logger" yaml:"enable_debug_logger"`
	ConnectionPoolSize    int           `json:"connection_pool_size" mapstructure:"connection_pool_size" ini:"connection_pool_size" yaml:"connection_pool_size"`
	ResponseHeaderTimeout time.Duration `json:"response_header_timeout" mapstructure:"response_header_timeout" ini:"response_header_timeout" yaml:"response_header_timeout"`
	RequestTimeout        time.Duration `json:"request_timeout" mapstructure:"request_timeout" ini:"request_timeout" yaml:"request_timeout"`
	DialTimeout           time.Duration `json:"dial_timeout" mapstructure:"dial_timeout" ini:"dial_timeout" yaml:"dial_timeout"`
	KeepAliveInterval     time.Duration `json:"keep_alive_interval" mapstructure:"keep_alive_interval" ini:"keep_alive_interval" yaml:"keep_alive_interval"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Elasticsearch) setDefault() {
	cv.SetDefault("elasticsearch.addrs", []string{"http://localhost:9200"})
	cv.SetDefault("elasticsearch.username", "")
	cv.SetDefault("elasticsearch.password", "")
	cv.SetDefault("elasticsearch.cloud_id", "")
	cv.SetDefault("elasticsearch.api_key", "")
	cv.SetDefault("elasticsearch.max_retries", 3)
	cv.SetDefault("elasticsearch.retry_on_status", []int{502, 503, 504})
	cv.SetDefault("elasticsearch.disable_retries", false)
	cv.SetDefault("elasticsearch.retry_backoff", true)
	cv.SetDefault("elasticsearch.retry_backoff_min", 1*time.Second)
	cv.SetDefault("elasticsearch.retry_backoff_max", 30*time.Second)
	cv.SetDefault("elasticsearch.compress", true)
	cv.SetDefault("elasticsearch.discovery_interval", 5*time.Minute)
	cv.SetDefault("elasticsearch.enable_metrics", false)
	cv.SetDefault("elasticsearch.enable_debug_logger", false)
	cv.SetDefault("elasticsearch.connection_pool_size", 0)    // Default is 0 (unlimited)
	cv.SetDefault("elasticsearch.response_header_timeout", 0) // 0 means no timeout
	cv.SetDefault("elasticsearch.request_timeout", 0)         // 0 means no timeout
	cv.SetDefault("elasticsearch.dial_timeout", 30*time.Second)
	cv.SetDefault("elasticsearch.keep_alive_interval", 15*time.Second)

	cv.SetDefault("elasticsearch.enable_tls", false)
	cv.SetDefault("elasticsearch.cert_file", "")
	cv.SetDefault("elasticsearch.key_file", "")
	cv.SetDefault("elasticsearch.ca_file", "")
	cv.SetDefault("elasticsearch.insecure_skip_verify", false)

	cv.SetDefault("elasticsearch.enable", false)
}
