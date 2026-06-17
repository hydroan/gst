package config

import (
	"time"
)

const (
	CASSANDRA_HOSTS              = "CASSANDRA_HOSTS"              //nolint:staticcheck
	CASSANDRA_PORT               = "CASSANDRA_PORT"               //nolint:staticcheck
	CASSANDRA_KEYSPACE           = "CASSANDRA_KEYSPACE"           //nolint:staticcheck
	CASSANDRA_USERNAME           = "CASSANDRA_USERNAME"           //nolint:staticcheck
	CASSANDRA_PASSWORD           = "CASSANDRA_PASSWORD"           //nolint:staticcheck
	CASSANDRA_CONSISTENCY        = "CASSANDRA_CONSISTENCY"        //nolint:staticcheck
	CASSANDRA_TIMEOUT            = "CASSANDRA_TIMEOUT"            //nolint:staticcheck
	CASSANDRA_CONNECT_TIMEOUT    = "CASSANDRA_CONNECT_TIMEOUT"    //nolint:staticcheck
	CASSANDRA_NUM_CONNS          = "CASSANDRA_NUM_CONNS"          //nolint:staticcheck
	CASSANDRA_PAGE_SIZE          = "CASSANDRA_PAGE_SIZE"          //nolint:staticcheck
	CASSANDRA_RETRY_POLICY       = "CASSANDRA_RETRY_POLICY"       //nolint:staticcheck
	CASSANDRA_RECONNECT_INTERVAL = "CASSANDRA_RECONNECT_INTERVAL" //nolint:staticcheck
	CASSANDRA_MAX_RETRY_COUNT    = "CASSANDRA_MAX_RETRY_COUNT"    //nolint:staticcheck

	CASSANDRA_ENABLE_TLS           = "CASSANDRA_ENABLE_TLS"           //nolint:staticcheck
	CASSANDRA_CERT_FILE            = "CASSANDRA_CERT_FILE"            //nolint:staticcheck
	CASSANDRA_KEY_FILE             = "CASSANDRA_KEY_FILE"             //nolint:staticcheck
	CASSANDRA_CA_FILE              = "CASSANDRA_CA_FILE"              //nolint:staticcheck
	CASSANDRA_INSECURE_SKIP_VERIFY = "CASSANDRA_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	CASSANDRA_ENABLE = "CASSANDRA_ENABLE" //nolint:staticcheck
)

type Cassandra struct {
	Hosts    []string `json:"hosts" mapstructure:"hosts" ini:"hosts" yaml:"hosts"`
	Port     int      `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Keyspace string   `json:"keyspace" mapstructure:"keyspace" ini:"keyspace" yaml:"keyspace"`
	Username string   `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password string   `json:"password" mapstructure:"password" ini:"password" yaml:"password"`

	Consistency       string        `json:"consistency" mapstructure:"consistency" ini:"consistency" yaml:"consistency"`
	Timeout           time.Duration `json:"timeout" mapstructure:"timeout" ini:"timeout" yaml:"timeout"`
	ConnectTimeout    time.Duration `json:"connect_timeout" mapstructure:"connect_timeout" ini:"connect_timeout" yaml:"connect_timeout"`
	NumConns          int           `json:"num_conns" mapstructure:"num_conns" ini:"num_conns" yaml:"num_conns"`
	PageSize          int           `json:"page_size" mapstructure:"page_size" ini:"page_size" yaml:"page_size"`
	RetryPolicy       string        `json:"retry_policy" mapstructure:"retry_policy" ini:"retry_policy" yaml:"retry_policy"`
	ReconnectInterval time.Duration `json:"reconnect_interval" mapstructure:"reconnect_interval" ini:"reconnect_interval" yaml:"reconnect_interval"`
	MaxRetryCount     int           `json:"max_retry_count" mapstructure:"max_retry_count" ini:"max_retry_count" yaml:"max_retry_count"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Cassandra) setDefault() {
	cv.SetDefault("cassandra.hosts", []string{"127.0.0.1"})
	cv.SetDefault("cassandra.port", 9042)
	cv.SetDefault("cassandra.keyspace", "")
	cv.SetDefault("cassandra.username", "")
	cv.SetDefault("cassandra.password", "")

	cv.SetDefault("cassandra.consistency", "QUORUM")
	cv.SetDefault("cassandra.timeout", 5*time.Second)
	cv.SetDefault("cassandra.connect_timeout", 5*time.Second)
	cv.SetDefault("cassandra.num_conns", 2)
	cv.SetDefault("cassandra.page_size", 5000)
	cv.SetDefault("cassandra.retry_policy", "default")
	cv.SetDefault("cassandra.reconnect_interval", 1*time.Second)
	cv.SetDefault("cassandra.max_retry_count", 3)

	cv.SetDefault("cassandra.enable_tls", false)
	cv.SetDefault("cassandra.cert_file", "")
	cv.SetDefault("cassandra.key_file", "")
	cv.SetDefault("cassandra.ca_file", "")
	cv.SetDefault("cassandra.insecure_skip_verify", false)

	cv.SetDefault("cassandra.enable", false)
}
