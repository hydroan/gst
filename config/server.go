package config

import (
	"time"

	"github.com/spf13/viper"
)

type (
	Mode string
)

const (
	Prod  = "prod"
	Stg   = "stg"
	Pre   = "pre"
	Test  = "test"
	Dev   = "dev"
	Local = "local"
)

const (
	SERVER_DOMAIN = "SERVER_DOMAIN" //nolint:staticcheck
	SERVER_MODE   = "SERVER_MODE"   //nolint:staticcheck
	SERVER_LISTEN = "SERVER_LISTEN" //nolint:staticcheck
	SERVER_PORT   = "SERVER_PORT"   //nolint:staticcheck

	SERVER_TRUSTED_PROXIES = "SERVER_TRUSTED_PROXIES" //nolint:staticcheck

	SERVER_READ_TIMEOUT  = "SERVER_READ_TIMEOUT"  //nolint:staticcheck
	SERVER_WRITE_TIMEOUT = "SERVER_WRITE_TIMEOUT" //nolint:staticcheck
	SERVER_IDLE_TIMEOUT  = "SERVER_IDLE_TIMEOUT"  //nolint:staticcheck

	SERVER_CIRCUIT_BREAKER_NAME         = "SERVER_CIRCUIT_BREAKER_NAME"         //nolint:staticcheck
	SERVER_CIRCUIT_BREAKER_MAX_REQUESTS = "SERVER_CIRCUIT_BREAKER_MAX_REQUESTS" //nolint:staticcheck
	SERVER_CIRCUIT_BREAKER_INTERVAL     = "SERVER_CIRCUIT_BREAKER_INTERVAL"     //nolint:staticcheck
	SERVER_CIRCUIT_BREAKER_TIMEOUT      = "SERVER_CIRCUIT_BREAKER_TIMEOUT"      //nolint:staticcheck
	SERVER_CIRCUIT_BREAKER_FAILURE_RATE = "SERVER_CIRCUIT_BREAKER_FAILURE_RATE" //nolint:staticcheck
	SERVER_CIRCUIT_BREAKER_MIN_REQUESTS = "SERVER_CIRCUIT_BREAKER_MIN_REQUESTS" //nolint:staticcheck
	SERVER_CIRCUIT_BREAKER_ENABLED      = "SERVER_CIRCUIT_BREAKER_ENABLED"      //nolint:staticcheck

	SERVER_CIRCULAR_BUFFER_SIZE_OPERATION_LOG = "SERVER_CIRCULAR_BUFFER_SIZE_OPERATION_LOG" //nolint:staticcheck
)

type Server struct {
	Mode   Mode   `json:"mode" mapstructure:"mode" ini:"mode" yaml:"mode"`
	Listen string `json:"listen" mapstructure:"listen" ini:"listen" yaml:"listen"`
	Port   int    `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Domain string `json:"domain" mapstructure:"domain" ini:"domain" yaml:"domain"`

	// TrustedProxies lists the CIDR blocks of the proxies allowed to report a
	// client address through X-Forwarded-For or X-Real-IP.
	//
	// It is empty by default, and an empty list trusts no peer: the client
	// address is then read off the connection, which the caller cannot
	// rewrite. A deployment behind a load balancer or ingress names that
	// network here — until it does, every client address it records, logs and
	// rate-limits on is the one the request asked for.
	TrustedProxies []string `json:"trusted_proxies" mapstructure:"trusted_proxies" ini:"trusted_proxies" yaml:"trusted_proxies"`

	ReadTimeout  time.Duration `json:"read_timeout" mapstructure:"read_timeout" ini:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" mapstructure:"write_timeout" ini:"write_timeout" yaml:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout" mapstructure:"idle_timeout" ini:"idle_timeout" yaml:"idle_timeout"`

	// Circuit breaker
	CircuitBreaker CircuitBreaker `json:"circuit_breaker" mapstructure:"circuit_breaker" ini:"circuit_breaker" yaml:"circuit_breaker"`

	// Circular buffer
	CircularBuffer CircularBuffer `json:"circular_buffer" mapstructure:"circular_buffer" ini:"circular_buffer" yaml:"circular_buffer"`
}

type CircuitBreaker struct {
	Name        string        `json:"name" mapstructure:"name" ini:"name" yaml:"name"`
	MaxRequests uint32        `json:"max_requests" mapstructure:"max_requests" ini:"max_requests" yaml:"max_requests"`
	Interval    time.Duration `json:"interval" mapstructure:"interval" ini:"interval" yaml:"interval"`
	Timeout     time.Duration `json:"timeout" mapstructure:"timeout" ini:"timeout" yaml:"timeout"`
	FailureRate float64       `json:"failure_rate" mapstructure:"failure_rate" ini:"failure_rate" yaml:"failure_rate"`
	MinRequests uint32        `json:"min_requests" mapstructure:"min_requests" ini:"min_requests" yaml:"min_requests"`
	Enabled     bool          `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}
type CircularBuffer struct {
	SizeOperationLog int64 `json:"size_operation_log" mapstructure:"size_operation_log" ini:"size" yaml:"size_operation_log"`
}

func (*Server) setDefault(v *viper.Viper) {
	v.SetDefault("server.mode", Dev)
	v.SetDefault("server.listen", "")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.domain", "")
	v.SetDefault("server.trusted_proxies", []string{})
	v.SetDefault("server.read_timeout", 15*time.Second)
	v.SetDefault("server.write_timeout", 15*time.Second)
	v.SetDefault("server.idle_timeout", 60*time.Second)

	// Circuit breaker defaults
	v.SetDefault("server.circuit_breaker.name", "backend-server")
	v.SetDefault("server.circuit_breaker.max_requests", uint32(100))
	v.SetDefault("server.circuit_breaker.interval", 10*time.Second)
	v.SetDefault("server.circuit_breaker.timeout", 30*time.Second)
	v.SetDefault("server.circuit_breaker.failure_rate", 0.5)
	v.SetDefault("server.circuit_breaker.min_requests", uint32(10))
	v.SetDefault("server.circuit_breaker.enabled", true)

	// Circular buffer defaults
	v.SetDefault("server.circular_buffer.size_operation_log", int64(10000))
}
