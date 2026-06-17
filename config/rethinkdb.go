package config

import "time"

const (
	RETHINKDB_HOSTS          = "RETHINKDB_HOSTS"          //nolint:staticcheck
	RETHINKDB_USERNAME       = "RETHINKDB_USERNAME"       //nolint:staticcheck
	RETHINKDB_PASSWORD       = "RETHINKDB_PASSWORD"       //nolint:staticcheck,gosec
	RETHINKDB_DATABASE       = "RETHINKDB_DATABASE"       //nolint:staticcheck
	RETHINKDB_DISCOVERY_HOST = "RETHINKDB_DISCOVERY_HOST" //nolint:staticcheck

	RETHINKDB_MAX_IDLE    = "RETHINKDB_MAX_IDLE"    //nolint:staticcheck
	RETHINKDB_MAX_OPEN    = "RETHINKDB_MAX_OPEN"    //nolint:staticcheck
	RETHINKDB_NUM_RETRIES = "RETHINKDB_NUM_RETRIES" //nolint:staticcheck

	RETHINKDB_CONNECT_TIMEOUT = "RETHINKDB_CONNECT_TIMEOUT" //nolint:staticcheck
	RETHINKDB_READ_TIMEOUT    = "RETHINKDB_READ_TIMEOUT"    //nolint:staticcheck
	RETHINKDB_WRITE_TIMEOUT   = "RETHINKDB_WRITE_TIMEOUT"   //nolint:staticcheck
	RETHINKDB_KEEP_ALIVE_TIME = "RETHINKDB_KEEP_ALIVE_TIME" //nolint:staticcheck

	RETHINKDB_ENABLE_TLS           = "RETHINKDB_ENABLE_TLS"           //nolint:staticcheck
	RETHINKDB_CERT_FILE            = "RETHINKDB_CERT_FILE"            //nolint:staticcheck
	RETHINKDB_KEY_FILE             = "RETHINKDB_KEY_FILE"             //nolint:staticcheck
	RETHINKDB_CA_FILE              = "RETHINKDB_CA_FILE"              //nolint:staticcheck
	RETHINKDB_INSECURE_SKIP_VERIFY = "RETHINKDB_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	RETHINKDB_ENABLE = "RETHINKDB_ENABLE" //nolint:staticcheck
)

type RethinkDB struct {
	Hosts         []string `json:"hosts" mapstructure:"hosts" ini:"hosts" yaml:"hosts"`
	Username      string   `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password      string   `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	Database      string   `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	DiscoveryHost bool     `json:"discovery_host" mapstructure:"discovery_host" ini:"discovery_host" yaml:"discovery_host"`

	MaxIdle    int `json:"max_idle" mapstructure:"max_idle" ini:"max_idle" yaml:"max_idle"`
	MaxOpen    int `json:"max_open" mapstructure:"max_open" ini:"max_open" yaml:"max_open"`
	NumRetries int `json:"num_retries" mapstructure:"num_retries" ini:"num_retries" yaml:"num_retries"`

	ConnectTimeout time.Duration `json:"connect_timeout" mapstructure:"connect_timeout" ini:"connect_timeout" yaml:"connect_timeout"`
	ReadTimeout    time.Duration `json:"read_timeout" mapstructure:"read_timeout" ini:"read_timeout" yaml:"read_timeout"`
	WriteTimeout   time.Duration `json:"write_timeout" mapstructure:"write_timeout" ini:"write_timeout" yaml:"write_timeout"`
	KeepAliveTime  time.Duration `json:"keep_alive_time" mapstructure:"keep_alive_time" ini:"keep_alive_time" yaml:"keep_alive_time"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*RethinkDB) setDefault() {
	cv.SetDefault("rethinkdb.hosts", "127.0.0.1:28015")
	cv.SetDefault("rethinkdb.username", "")
	cv.SetDefault("rethinkdb.password", "")
	cv.SetDefault("rethinkdb.database", "")
	cv.SetDefault("rethinkdb.discovery_host", false)

	cv.SetDefault("rethinkdb.max_idle", 10)
	cv.SetDefault("rethinkdb.max_open", 100)
	cv.SetDefault("rethinkdb.num_retries", 3)

	cv.SetDefault("rethinkdb.connect_timeout", 5*time.Second)
	cv.SetDefault("rethinkdb.read_timeout", 10*time.Second)
	cv.SetDefault("rethinkdb.write_timeout", 10*time.Second)
	cv.SetDefault("rethinkdb.keep_alive_time", 30*time.Second)

	cv.SetDefault("rethinkdb.enable_tls", false)
	cv.SetDefault("rethinkdb.cert_file", "")
	cv.SetDefault("rethinkdb.key_file", "")
	cv.SetDefault("rethinkdb.ca_file", "")
	cv.SetDefault("rethinkdb.insecure_skip_verify", false)

	cv.SetDefault("rethinkdb.enable", false)
}
