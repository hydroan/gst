package config

import (
	"time"

	"github.com/spf13/viper"
)

const (
	GRPC_LISTEN            = "GRPC_LISTEN"
	GRPC_PORT              = "GRPC_PORT"
	GRPC_TLS_ENABLED       = "GRPC_TLS_ENABLED"
	GRPC_CERT_FILE         = "GRPC_CERT_FILE"
	GRPC_KEY_FILE          = "GRPC_KEY_FILE"
	GRPC_REFLECTION        = "GRPC_REFLECTION"
	GRPC_KEEPALIVE_TIME    = "GRPC_KEEPALIVE_TIME"
	GRPC_KEEPALIVE_TIMEOUT = "GRPC_KEEPALIVE_TIMEOUT"
)

// GRPC configures the gRPC listener serving the models that declare GRPC(),
// beside the HTTP listener. It opens only when a service is registered on
// it, so a project without such models exposes no port and has nothing to
// switch off.
type GRPC struct {
	// Listen is the address the listener binds, every interface when empty,
	// and Port its port, 9090 by default.
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

	// KeepaliveTime is how long a connection may stay idle before the server
	// pings it, and KeepaliveTimeout how long it then waits for the answer
	// before closing the connection. Zero leaves grpc-go's own defaults, two
	// hours and twenty seconds.
	KeepaliveTime    time.Duration `json:"keepalive_time" mapstructure:"keepalive_time" ini:"keepalive_time" yaml:"keepalive_time"`
	KeepaliveTimeout time.Duration `json:"keepalive_timeout" mapstructure:"keepalive_timeout" ini:"keepalive_timeout" yaml:"keepalive_timeout"`
}

func (*GRPC) setDefault(v *viper.Viper) {
	v.SetDefault("grpc.listen", "")
	v.SetDefault("grpc.port", 9090)
	v.SetDefault("grpc.tls_enabled", false)
	v.SetDefault("grpc.cert_file", "")
	v.SetDefault("grpc.key_file", "")
	v.SetDefault("grpc.reflection", true)
	v.SetDefault("grpc.keepalive_time", 0)
	v.SetDefault("grpc.keepalive_timeout", 0)
}
