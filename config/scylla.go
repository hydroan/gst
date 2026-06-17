package config

import "time"

type Consistency string

const (
	ConsistencyAny         Consistency = "any"
	ConsistencyOne         Consistency = "one"
	ConsistencyTwo         Consistency = "two"
	ConsistencyThree       Consistency = "three"
	ConsistencyQuorum      Consistency = "quorum"
	ConsistencyAll         Consistency = "all"
	ConsistencyLocalQuorum Consistency = "local_quorum"
	ConsistencyEachQuorum  Consistency = "each_quorum"
	ConsistencyLocalOne    Consistency = "local_one"
)

type RetryPolicy string

const (
	RetryPolicySimple                 RetryPolicy = "simple"
	RetryPolicyDowngradingConsistency RetryPolicy = "downgrading_consistency"
	RetryPolicyExponentialBackoff     RetryPolicy = "exponential_backoff"
)

type ReconnectPolicy string

const (
	ReconnectPolicyConstant    ReconnectPolicy = "constant"
	ReconnectPolicyExponential ReconnectPolicy = "exponential"
)

const (
	SCYLLA_HOSTS           = "SCYLLA_HOSTS"           //nolint:staticcheck
	SCYLLA_USERNAME        = "SCYLLA_USERNAME"        //nolint:staticcheck
	SCYLLA_PASSWORD        = "SCYLLA_PASSWORD"        //nolint:staticcheck
	SCYLLA_KEYSPACE        = "SCYLLA_KEYSPACE"        //nolint:staticcheck
	SCYLLA_CONSISTENCY     = "SCYLLA_CONSISTENCY"     //nolint:staticcheck
	SCYLLA_NUM_CONNS       = "SCYLLA_NUM_CONNS"       //nolint:staticcheck
	SCYLLA_CONNECT_TIMEOUT = "SCYLLA_CONNECT_TIMEOUT" //nolint:staticcheck
	SCYLLA_TIMEOUT         = "SCYLLA_TIMEOUT"         //nolint:staticcheck
	SCYLLA_PAGE_SIZE       = "SCYLLA_PAGE_SIZE"       //nolint:staticcheck

	SCYLLA_RETRY_POLICY       = "SCYLLA_RETRY_POLICY"       //nolint:staticcheck
	SCYLLA_RETRY_NUM_RETRIES  = "SCYLLA_RETRY_NUM_RETRIES"  //nolint:staticcheck
	SCYLLA_RETRY_MIN_INTERVAL = "SCYLLA_RETRY_MIN_INTERVAL" //nolint:staticcheck
	SCYLLA_RETRY_MAX_INTERVAL = "SCYLLA_RETRY_MAX_INTERVAL" //nolint:staticcheck

	SCYLLA_RECONNECT_POLICY            = "SCYLLA_RECONNECT_POLICY"            //nolint:staticcheck
	SCYLLA_RECONNECT_MAX_RETRIES       = "SCYLLA_RECONNECT_MAX_RETRIES"       //nolint:staticcheck
	SCYLLA_RECONNECT_INITIAL_INTERVAL  = "SCYLLA_RECONNECT_INITIAL_INTERVAL"  //nolint:staticcheck
	SCYLLA_RECONNECT_MAX_INTERVAL      = "SCYLLA_RECONNECT_MAX_INTERVAL"      //nolint:staticcheck
	SCYLLA_RECONNECT_CONSTANT_INTERVAL = "SCYLLA_RECONNECT_CONSTANT_INTERVAL" //nolint:staticcheck

	SCYLLA_ENABLE_TRACING       = "SCYLLA_ENABLE_TRACING"       //nolint:staticcheck
	SCYLLA_ENABLE_TLS           = "SCYLLA_ENABLE_TLS"           //nolint:staticcheck
	SCYLLA_CERT_FILE            = "SCYLLA_CERT_FILE"            //nolint:staticcheck
	SCYLLA_KEY_FILE             = "SCYLLA_KEY_FILE"             //nolint:staticcheck
	SCYLLA_CA_FILE              = "SCYLLA_CA_FILE"              //nolint:staticcheck
	SCYLLA_INSECURE_SKIP_VERIFY = "SCYLLA_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	SCYLLA_ENABLE = "SCYLLA_ENABLE" //nolint:staticcheck
)

type Scylla struct {
	Hosts       []string    `json:"hosts" mapstructure:"hosts" ini:"hosts" yaml:"hosts"`
	Username    string      `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password    string      `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	Keyspace    string      `json:"keyspace" mapstructure:"keyspace" ini:"keyspace" yaml:"keyspace"`
	Consistency Consistency `json:"consistency" mapstructure:"consistency" ini:"consistency" yaml:"consistency"`
	NumConns    int         `json:"num_conns" mapstructure:"num_conns" ini:"num_conns" yaml:"num_conns"`

	ConnectTimeout time.Duration `json:"connect_timeout" mapstructure:"connect_timeout" ini:"connect_timeout" yaml:"connect_timeout"`
	Timeout        time.Duration `json:"timeout" mapstructure:"timeout" ini:"timeout" yaml:"timeout"`
	PageSize       int           `json:"page_size" mapstructure:"page_size" ini:"page_size" yaml:"page_size"`

	RetryPolicy      RetryPolicy   `json:"retry_policy" mapstructure:"retry_policy" ini:"retry_policy" yaml:"retry_policy"`
	RetryNumRetries  int           `json:"retry_num_retries" mapstructure:"retry_num_retries" ini:"retry_num_retries" yaml:"retry_num_retries"`
	RetryMinInterval time.Duration `json:"retry_min_interval" mapstructure:"retry_min_interval" ini:"retry_min_interval" yaml:"retry_min_interval"`
	RetryMaxInterval time.Duration `json:"retry_max_interval" mapstructure:"retry_max_interval" ini:"retry_max_interval" yaml:"retry_max_interval"`

	ReconnectPolicy           ReconnectPolicy `json:"reconnect_policy" mapstructure:"reconnect_policy" ini:"reconnect_policy" yaml:"reconnect_policy"`
	ReconnectMaxRetries       int             `json:"reconnect_max_retries" mapstructure:"reconnect_max_retries" ini:"reconnect_max_retries" yaml:"reconnect_max_retries"`
	ReconnectInitialInterval  time.Duration   `json:"reconnect_initial_interval" mapstructure:"reconnect_initial_interval" ini:"reconnect_initial_interval" yaml:"reconnect_initial_interval"`
	ReconnectMaxInterval      time.Duration   `json:"reconnect_max_interval" mapstructure:"reconnect_max_interval" ini:"reconnect_max_interval" yaml:"reconnect_max_interval"`
	ReconnectConstantInterval time.Duration   `json:"reconnect_constant_interval" mapstructure:"reconnect_constant_interval" ini:"reconnect_constant_interval" yaml:"reconnect_constant_interval"`

	EnableTracing      bool   `json:"enable_tracing" mapstructure:"enable_tracing" ini:"enable_tracing" yaml:"enable_tracing"`
	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Scylla) setDefault() {
	cv.SetDefault("scylla.hosts", []string{"127.0.0.1:9042"})
	cv.SetDefault("scylla.username", "")
	cv.SetDefault("scylla.password", "")
	cv.SetDefault("scylla.keyspace", "")
	cv.SetDefault("scylla.consistency", ConsistencyQuorum)
	cv.SetDefault("scylla.num_conns", 0)

	cv.SetDefault("scylla.connect_timeout", 5*time.Second)
	cv.SetDefault("scylla.timeout", 30*time.Second)
	cv.SetDefault("scylla.page_size", 5000)

	cv.SetDefault("scylla.retry_policy", "")
	cv.SetDefault("scylla.retry_num_retries", 5)
	cv.SetDefault("scylla.retry_min_interval", 100*time.Millisecond)
	cv.SetDefault("scylla.retry_max_interval", 5*time.Second)

	cv.SetDefault("scylla.reconnect_policy", "")
	cv.SetDefault("scylla.reconnect_max_retries", 10)
	cv.SetDefault("scylla.reconnect_initial_interval", 100*time.Millisecond)
	cv.SetDefault("scylla.reconnect_max_interval", 10*time.Second)
	cv.SetDefault("scylla.reconnect_constant_interval", 1*time.Second)

	cv.SetDefault("scylla.enable_tracing", false)
	cv.SetDefault("scylla.enable_tls", false)
	cv.SetDefault("scylla.cert_file", "")
	cv.SetDefault("scylla.key_file", "")
	cv.SetDefault("scylla.ca_file", "")
	cv.SetDefault("scylla.insecure_skip_verify", false)

	cv.SetDefault("scylla.enable", false)
}
