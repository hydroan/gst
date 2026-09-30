package config

import (
	"time"

	"github.com/spf13/viper"
)

const (
	GRPC_LISTEN                          = "GRPC_LISTEN"
	GRPC_PORT                            = "GRPC_PORT"
	GRPC_TLS_ENABLED                     = "GRPC_TLS_ENABLED"
	GRPC_CERT_FILE                       = "GRPC_CERT_FILE"
	GRPC_KEY_FILE                        = "GRPC_KEY_FILE"
	GRPC_REFLECTION                      = "GRPC_REFLECTION"
	GRPC_KEEPALIVE_TIME                  = "GRPC_KEEPALIVE_TIME"
	GRPC_KEEPALIVE_TIMEOUT               = "GRPC_KEEPALIVE_TIMEOUT"
	GRPC_KEEPALIVE_MIN_TIME              = "GRPC_KEEPALIVE_MIN_TIME"
	GRPC_KEEPALIVE_PERMIT_WITHOUT_STREAM = "GRPC_KEEPALIVE_PERMIT_WITHOUT_STREAM"
	GRPC_MAX_CONNECTION_AGE              = "GRPC_MAX_CONNECTION_AGE"
	GRPC_MAX_CONNECTION_AGE_GRACE        = "GRPC_MAX_CONNECTION_AGE_GRACE"
	GRPC_MAX_RECV_MSG_SIZE               = "GRPC_MAX_RECV_MSG_SIZE"
)

// GRPC configures the gRPC listener serving the models that declare GRPC(),
// beside the HTTP listener. It opens only when a service is registered on
// it, so a project without such models exposes no port and has nothing to
// switch off.
type GRPC struct {
	// Listen is the address the listener binds, every interface when empty,
	// and Port its port, 8081 by default, next to the HTTP listener's 8080.
	Listen string `json:"listen" mapstructure:"listen" ini:"listen" yaml:"listen"`
	Port   int    `json:"port" mapstructure:"port" ini:"port" yaml:"port"`

	// TLSEnabled serves TLS with the certificate in CertFile and the key in
	// KeyFile. Off by default: the listener is plaintext the way the HTTP
	// listener is, for an ingress that terminates TLS in front of it.
	TLSEnabled bool   `json:"tls_enabled" mapstructure:"tls_enabled" ini:"tls_enabled" yaml:"tls_enabled"`
	CertFile   string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile    string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`

	// Reflection registers the server reflection service, through which
	// grpcurl and its kind list the services and their messages. On by
	// default: the definitions are the project's own, committed under pb/.
	Reflection bool `json:"reflection" mapstructure:"reflection" ini:"reflection" yaml:"reflection"`

	// The keys below are grpc-go's own server parameters, named as grpc-go
	// names them; a zero value leaves grpc-go's own default for it. A
	// negative duration, or a KeepaliveTime or MaxConnectionAge under a
	// second, is refused as the process starts (see grpcserver.Run).

	// KeepaliveTime is how long a connection may stay idle before the server
	// pings it, and KeepaliveTimeout how long it then waits for the answer
	// before closing the connection. Zero leaves grpc-go's own defaults, two
	// hours and twenty seconds.
	KeepaliveTime    time.Duration `json:"keepalive_time" mapstructure:"keepalive_time" ini:"keepalive_time" yaml:"keepalive_time"`
	KeepaliveTimeout time.Duration `json:"keepalive_timeout" mapstructure:"keepalive_timeout" ini:"keepalive_timeout" yaml:"keepalive_timeout"`

	// KeepaliveMinTime is the least time a client must leave between its own
	// pings, and KeepalivePermitWithoutStream whether it may ping at all
	// with no call on the connection; a client breaking the policy is sent
	// away with too_many_pings and its connection closed, so a client's
	// keepalive is set to match. Zero and false leave grpc-go's own policy,
	// five minutes and no pings between calls.
	KeepaliveMinTime             time.Duration `json:"keepalive_min_time" mapstructure:"keepalive_min_time" ini:"keepalive_min_time" yaml:"keepalive_min_time"`
	KeepalivePermitWithoutStream bool          `json:"keepalive_permit_without_stream" mapstructure:"keepalive_permit_without_stream" ini:"keepalive_permit_without_stream" yaml:"keepalive_permit_without_stream"`

	// MaxConnectionAge is how long a connection may live before the server
	// tells its client to reconnect, spread by a tenth either way so that
	// the connections do not all end at once: through a balancer that
	// spreads connections rather than calls, a Kubernetes Service for one,
	// a client reconnecting is how the replicas started since get their
	// share. MaxConnectionAgeGrace is how long the calls on the connection
	// then get to finish before it is closed. Zero leaves grpc-go's own
	// default, a connection living as long as its client keeps it.
	MaxConnectionAge      time.Duration `json:"max_connection_age" mapstructure:"max_connection_age" ini:"max_connection_age" yaml:"max_connection_age"`
	MaxConnectionAgeGrace time.Duration `json:"max_connection_age_grace" mapstructure:"max_connection_age_grace" ini:"max_connection_age_grace" yaml:"max_connection_age_grace"`

	// MaxRecvMsgSize is the largest message a call may carry to the server,
	// a size with its unit, parsed with github.com/dustin/go-humanize like
	// logger.http_body.max_body_size: "8MiB" is 8 times 1024 squared bytes,
	// "8MB" 8 times 1000 squared; a larger message is answered
	// ResourceExhausted. Empty leaves grpc-go's own limit, 4MiB.
	MaxRecvMsgSize string `json:"max_recv_msg_size" mapstructure:"max_recv_msg_size" ini:"max_recv_msg_size" yaml:"max_recv_msg_size"`
}

func (*GRPC) setDefault(v *viper.Viper) {
	v.SetDefault("grpc.listen", "")
	v.SetDefault("grpc.port", 8081)
	v.SetDefault("grpc.tls_enabled", false)
	v.SetDefault("grpc.cert_file", "")
	v.SetDefault("grpc.key_file", "")
	v.SetDefault("grpc.reflection", true)
	v.SetDefault("grpc.keepalive_time", 0)
	v.SetDefault("grpc.keepalive_timeout", 0)
	v.SetDefault("grpc.keepalive_min_time", 0)
	v.SetDefault("grpc.keepalive_permit_without_stream", false)
	v.SetDefault("grpc.max_connection_age", 0)
	v.SetDefault("grpc.max_connection_age_grace", 0)
	v.SetDefault("grpc.max_recv_msg_size", "")
}
