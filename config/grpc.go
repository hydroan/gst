package config

import "time"

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

	GRPC_ENABLE_TLS          = "GRPC_ENABLE_TLS"          //nolint:staticcheck
	GRPC_CERT_FILE           = "GRPC_CERT_FILE"           //nolint:staticcheck
	GRPC_KEY_FILE            = "GRPC_KEY_FILE"            //nolint:staticcheck
	GRPC_CA_FILE             = "GRPC_CA_FILE"             //nolint:staticcheck
	GRPC_ENABLE_REFLECTION   = "GRPC_ENABLE_REFLECTION"   //nolint:staticcheck
	GRPC_ENABLE_HEALTH_CHECK = "GRPC_ENABLE_HEALTH_CHECK" //nolint:staticcheck

	GRPC_ENABLE = "GRPC_ENABLE" //nolint:staticcheck
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

	EnableTLS         bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile          string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile           string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile            string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	EnableReflection  bool   `json:"enable_reflection" mapstructure:"enable_reflection" ini:"enable_reflection" yaml:"enable_reflection"`
	EnableHealthCheck bool   `json:"enable_health_check" mapstructure:"enable_health_check" ini:"enable_health_check" yaml:"enable_health_check"`
	Enable            bool   `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Grpc) setDefault() {
	cv.SetDefault("grpc.listen", "127.0.0.1")
	cv.SetDefault("grpc.port", 9090)
	cv.SetDefault("grpc.max_recv_msg_size", 4*1024*1024) // 4MB
	cv.SetDefault("grpc.max_send_msg_size", 4*1024*1024) // 4MB
	cv.SetDefault("grpc.initial_window_size", 0)         // Use gRPC default
	cv.SetDefault("grpc.initial_conn_window_size", 0)    // Use gRPC default

	cv.SetDefault("grpc.keepalive_time", 30*time.Second)
	cv.SetDefault("grpc.keepalive_timeout", 10*time.Second)
	cv.SetDefault("grpc.max_connection_idle", 15*time.Second)
	cv.SetDefault("grpc.max_connection_age", 30*time.Second)
	cv.SetDefault("grpc.max_connection_age_grace", 5*time.Second)

	cv.SetDefault("grpc.enable_tls", false)
	cv.SetDefault("grpc.cert_file", "")
	cv.SetDefault("grpc.key_file", "")
	cv.SetDefault("grpc.ca_file", "")
	cv.SetDefault("grpc.enable_reflection", false)
	cv.SetDefault("grpc.enable_health_check", true)

	cv.SetDefault("grpc.enable", false)
}
