package config

import (
	"runtime"
	"time"

	"github.com/hydroan/gst/types/consts"
)

const (
	REDIS_ADDR         = "REDIS_ADDR"         //nolint:staticcheck
	REDIS_ADDRS        = "REDIS_ADDRS"        //nolint:staticcheck
	REDIS_DB           = "REDIS_DB"           //nolint:staticcheck
	REDIS_PASSWORD     = "REDIS_PASSWORD"     //nolint:staticcheck
	REDIS_NAMESPACE    = "REDIS_NAMESPACE"    //nolint:staticcheck
	REDIS_POOL_SIZE    = "REDIS_POOL_SIZE"    //nolint:staticcheck
	REDIS_EXPIRATION   = "REDIS_EXPIRATION"   //nolint:staticcheck
	REDIS_CLUSTER_MODE = "REDIS_CLUSTER_MODE" //nolint:staticcheck

	REDIS_DIAL_TIMEOUT      = "REDIS_DIAL_TIMEOUT"      //nolint:staticcheck
	REDIS_READ_TIMEOUT      = "REDIS_READ_TIMEOUT"      //nolint:staticcheck
	REDIS_WRITE_TIMEOUT     = "REDIS_WRITE_TIMEOUT"     //nolint:staticcheck
	REDIS_MIN_IDLE_CONNS    = "REDIS_MIN_IDLE_CONNS"    //nolint:staticcheck
	REDIS_MAX_RETRIES       = "REDIS_MAX_RETRIES"       //nolint:staticcheck
	REDIS_MIN_RETRY_BACKOFF = "REDIS_MIN_RETRY_BACKOFF" //nolint:staticcheck
	REDIS_MAX_RETRY_BACKOFF = "REDIS_MAX_RETRY_BACKOFF" //nolint:staticcheck

	REDIS_ENABLE_TLS           = "REDIS_ENABLE_TLS"           //nolint:staticcheck
	REDIS_CERT_FILE            = "REDIS_CERT_FILE"            //nolint:staticcheck
	REDIS_KEY_FILE             = "REDIS_KEY_FILE"             //nolint:staticcheck
	REDIS_CA_FILE              = "REDIS_CA_FILE"              //nolint:staticcheck
	REDIS_INSECURE_SKIP_VERIFY = "REDIS_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	REDIS_ENABLE = "REDIS_ENABLE" //nolint:staticcheck
)

type Redis struct {
	Addr        string        `json:"addr" mapstructure:"addr" ini:"addr" yaml:"addr"`
	Addrs       []string      `json:"addrs" mapstructure:"addrs" ini:"addrs" yaml:"addrs"`
	DB          int           `json:"db" mapstructure:"db" ini:"db" yaml:"db"`
	Password    string        `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	Namespace   string        `json:"namespace" mapstructure:"namespace" ini:"namespace" yaml:"namespace"`
	PoolSize    int           `json:"pool_size" mapstructure:"pool_size" ini:"pool_size" yaml:"pool_size"`
	Expiration  time.Duration `json:"expiration" mapstructure:"expiration" ini:"expiration" yaml:"expiration"`
	ClusterMode bool          `json:"cluster_mode" mapstructure:"cluster_mode" ini:"cluster_mode" yaml:"cluster_mode"`

	DialTimeout     time.Duration `json:"dial_timeout" mapstructure:"dial_timeout" ini:"dial_timeout" yaml:"dial_timeout"`
	ReadTimeout     time.Duration `json:"read_timeout" mapstructure:"read_timeout" ini:"read_timeout" yaml:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout" mapstructure:"write_timeout" ini:"write_timeout" yaml:"write_timeout"`
	MinIdleConns    int           `json:"min_idle_conns" mapstructure:"min_idle_conns" ini:"min_idle_conns" yaml:"min_idle_conns"`
	MaxRetries      int           `json:"max_retries" mapstructure:"max_retries" ini:"max_retries" yaml:"max_retries"`
	MinRetryBackoff time.Duration `json:"min_retry_backoff" mapstructure:"min_retry_backoff" ini:"min_retry_backoff" yaml:"min_retry_backoff"`
	MaxRetryBackoff time.Duration `json:"max_retry_backoff" mapstructure:"max_retry_backoff" ini:"max_retry_backoff" yaml:"max_retry_backoff"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Redis) setDefault() {
	cv.SetDefault("redis.addr", "127.0.0.1:6379")
	cv.SetDefault("redis.addrs", []string{"127.0.0.1:6379"})
	cv.SetDefault("redis.db", 0)
	cv.SetDefault("redis.password", "")
	cv.SetDefault("redis.pool_size", runtime.NumCPU())
	cv.SetDefault("redis.namespace", consts.FrameworkName)
	cv.SetDefault("redis.expiration", 0)
	cv.SetDefault("redis.cluster_mode", false)

	cv.SetDefault("redis.dial_timeout", 0)
	cv.SetDefault("redis.read_timeout", 0)
	cv.SetDefault("redis.write_timeout", 0)
	cv.SetDefault("redis.min_idle_conns", 0)
	cv.SetDefault("redis.max_retries", 0)
	cv.SetDefault("redis.min_retry_backoff", 0)
	cv.SetDefault("redis.max_retry_backoff", 0)

	cv.SetDefault("redis.enable_tls", false)
	cv.SetDefault("redis.cert_file", "")
	cv.SetDefault("redis.key_file", 0)
	cv.SetDefault("redis.ca_file", "")
	cv.SetDefault("redis.insecure_skip_verify", false)

	cv.SetDefault("redis.enable", false)
}
