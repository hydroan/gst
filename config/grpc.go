package config

import (
	"time"

	"github.com/spf13/viper"
)

const (
	GRPC_LISTEN                   = "GRPC_LISTEN"                   //nolint:staticcheck
	GRPC_PORT                     = "GRPC_PORT"                     //nolint:staticcheck
	GRPC_MAX_RECV_MSG_SIZE        = "GRPC_MAX_RECV_MSG_SIZE"        //nolint:staticcheck
	GRPC_MAX_SEND_MSG_SIZE        = "GRPC_MAX_SEND_MSG_SIZE"        //nolint:staticcheck
	GRPC_INITIAL_WINDOW_SIZE      = "GRPC_INITIAL_WINDOW_SIZE"      //nolint:staticcheck
	GRPC_INITIAL_CONN_WINDOW_SIZE = "GRPC_INITIAL_CONN_WINDOW_SIZE" //nolint:staticcheck

	GRPC_KEEPALIVE_TIME           = "GRPC_KEEPALIVE_TIME"           //nolint:staticcheck
	GRPC_KEEPALIVE_TIMEOUT        = "GRPC_KEEPALIVE_TIMEOUT"        //nolint:staticcheck
	GRPC_MAX_CONNECTION_IDLE      = "GRPC_MAX_CONNECTION_IDLE"      //nolint:staticcheck
	GRPC_MAX_CONNECTION_AGE       = "GRPC_MAX_CONNECTION_AGE"       //nolint:staticcheck
	GRPC_MAX_CONNECTION_AGE_GRACE = "GRPC_MAX_CONNECTION_AGE_GRACE" //nolint:staticcheck

	GRPC_TLS_ENABLED          = "GRPC_TLS_ENABLED"          //nolint:staticcheck
	GRPC_CERT_FILE            = "GRPC_CERT_FILE"            //nolint:staticcheck
	GRPC_KEY_FILE             = "GRPC_KEY_FILE"             //nolint:staticcheck
	GRPC_CA_FILE              = "GRPC_CA_FILE"              //nolint:staticcheck
	GRPC_REFLECTION_ENABLED   = "GRPC_REFLECTION_ENABLED"   //nolint:staticcheck
	GRPC_HEALTH_CHECK_ENABLED = "GRPC_HEALTH_CHECK_ENABLED" //nolint:staticcheck

	GRPC_ENABLED = "GRPC_ENABLED" //nolint:staticcheck
)

type Grpc struct {
	Listen                string `json:"listen" mapstructure:"listen" ini:"listen" yaml:"listen" default:"127.0.0.1"`
	Port                  int    `json:"port" mapstructure:"port" ini:"port" yaml:"port" default:"11500"`
	MaxRecvMsgSize        int    `json:"max_recv_msg_size" mapstructure:"max_recv_msg_size" ini:"max_recv_msg_size" yaml:"max_recv_msg_size"`
	MaxSendMsgSize        int    `json:"max_send_msg_size" mapstructure:"max_send_msg_size" ini:"max_send_msg_size" yaml:"max_send_msg_size"`
	InitialWindowSize     int32  `json:"initial_window_size" mapstructure:"initial_window_size" ini:"initial_window_size" yaml:"initial_window_size"`
	InitialConnWindowSize int32  `json:"initial_conn_window_size" mapstructure:"initial_conn_window_size" ini:"initial_conn_window_size" yaml:"initial_conn_window_size"`

	KeepaliveTime         time.Duration `json:"keepalive_time" mapstructure:"keepalive_time" ini:"keepalive_time" yaml:"keepalive_time"`
	KeepaliveTimeout      time.Duration `json:"keepalive_timeout" mapstructure:"keepalive_timeout" ini:"keepalive_timeout" yaml:"keepalive_timeout"`
	MaxConnectionIdle     time.Duration `json:"max_connection_idle" mapstructure:"max_connection_idle" ini:"max_connection_idle" yaml:"max_connection_idle"`
	MaxConnectionAge      time.Duration `json:"max_connection_age" mapstructure:"max_connection_age" ini:"max_connection_age" yaml:"max_connection_age"`
	MaxConnectionAgeGrace time.Duration `json:"max_connection_age_grace" mapstructure:"max_connection_age_grace" ini:"max_connection_age_grace" yaml:"max_connection_age_grace"`

	TLSEnabled         bool   `json:"tls_enabled" mapstructure:"tls_enabled" ini:"tls_enabled" yaml:"tls_enabled"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	ReflectionEnabled  bool   `json:"reflection_enabled" mapstructure:"reflection_enabled" ini:"reflection_enabled" yaml:"reflection_enabled"`
	HealthCheckEnabled bool   `json:"health_check_enabled" mapstructure:"health_check_enabled" ini:"health_check_enabled" yaml:"health_check_enabled"`
	Enabled            bool   `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}

func (*Grpc) setDefault(v *viper.Viper) {
	v.SetDefault("grpc.listen", "127.0.0.1")
	v.SetDefault("grpc.port", 9090)
	v.SetDefault("grpc.max_recv_msg_size", 4*1024*1024) // 4MB
	v.SetDefault("grpc.max_send_msg_size", 4*1024*1024) // 4MB
	v.SetDefault("grpc.initial_window_size", 0)         // Use gRPC default
	v.SetDefault("grpc.initial_conn_window_size", 0)    // Use gRPC default

	v.SetDefault("grpc.keepalive_time", 30*time.Second)
	v.SetDefault("grpc.keepalive_timeout", 10*time.Second)
	v.SetDefault("grpc.max_connection_idle", 15*time.Second)
	v.SetDefault("grpc.max_connection_age", 30*time.Second)
	v.SetDefault("grpc.max_connection_age_grace", 5*time.Second)

	v.SetDefault("grpc.tls_enabled", false)
	v.SetDefault("grpc.cert_file", "")
	v.SetDefault("grpc.key_file", "")
	v.SetDefault("grpc.ca_file", "")
	v.SetDefault("grpc.reflection_enabled", false)
	v.SetDefault("grpc.health_check_enabled", true)

	v.SetDefault("grpc.enabled", false)
}
