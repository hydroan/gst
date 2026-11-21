package config

import (
	"time"

	"github.com/spf13/viper"
)

const (
	MYSQL_HOST     = "MYSQL_HOST"
	MYSQL_PORT     = "MYSQL_PORT"
	MYSQL_DATABASE = "MYSQL_DATABASE"
	MYSQL_USERNAME = "MYSQL_USERNAME"
	MYSQL_PASSWORD = "MYSQL_PASSWORD"

	MYSQL_DIAL_TIMEOUT  = "MYSQL_DIAL_TIMEOUT"
	MYSQL_READ_TIMEOUT  = "MYSQL_READ_TIMEOUT"
	MYSQL_WRITE_TIMEOUT = "MYSQL_WRITE_TIMEOUT"

	MYSQL_REPLICAS = "MYSQL_REPLICAS"

	MYSQL_STRICT = "MYSQL_STRICT"

	MYSQL_TLS_ENABLED          = "MYSQL_TLS_ENABLED"
	MYSQL_CERT_FILE            = "MYSQL_CERT_FILE"
	MYSQL_KEY_FILE             = "MYSQL_KEY_FILE"
	MYSQL_CA_FILE              = "MYSQL_CA_FILE"
	MYSQL_INSECURE_SKIP_VERIFY = "MYSQL_INSECURE_SKIP_VERIFY"

	MYSQL_ENABLED = "MYSQL_ENABLED"
)

type MySQL struct {
	Host     string `json:"host" mapstructure:"host" ini:"host" yaml:"host"`
	Port     uint   `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Database string `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	Username string `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password string `json:"password" mapstructure:"password" ini:"password" yaml:"password"`

	// DialTimeout bounds establishing a TCP connection to the server. Without
	// it a dial against a host that drops packets instead of refusing them
	// blocks until the OS gives up, minutes later. A healthy handshake takes
	// milliseconds, so the 10s default only ever cuts off connections that
	// were never going to succeed. Zero disables the bound.
	DialTimeout time.Duration `json:"dial_timeout" mapstructure:"dial_timeout" ini:"dial_timeout" yaml:"dial_timeout"`
	// ReadTimeout and WriteTimeout bound single socket reads and writes on an
	// established connection. They default to zero (disabled) on purpose: a
	// connection-level I/O deadline also kills legitimately slow queries and
	// large result sets, and a per-query bound belongs to the caller's
	// context deadline instead.
	ReadTimeout  time.Duration `json:"read_timeout" mapstructure:"read_timeout" ini:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" mapstructure:"write_timeout" ini:"write_timeout" yaml:"write_timeout"`

	// Replicas lists read-replica endpoints as host:port entries (comma
	// separated in ini). Every other connection setting — credentials,
	// database, timeouts, the TLS settings, the utf8mb4 charset, the UTC wire
	// location — is shared with the primary, so a replica differs by address
	// only. Configuring replicas makes framework reads eligible for replica
	// routing, but never moves them by default: reads go to a replica only
	// where a model declares PreferReplica or a call site opts in with
	// WithReplica. Replication itself, replica health, and failover belong to
	// the infrastructure; prefer a single load-balanced read endpoint where
	// one exists.
	Replicas []string `json:"replicas" mapstructure:"replicas" ini:"replicas" yaml:"replicas"`

	// Strict adds STRICT_TRANS_TABLES to the sql_mode of every connection,
	// keeping the modes the server is configured with: a value too long for
	// its column is then refused, which the framework answers 400 with,
	// instead of stored cut short with a warning. On by default; off leaves
	// the server's modes alone, and a too-long value is stored cut short
	// wherever the server itself is not strict.
	Strict bool `json:"strict" mapstructure:"strict" ini:"strict" yaml:"strict"`

	// TLSEnabled encrypts every connection to the server, the primary's, the
	// replicas' and the one a migration runs over, and holds the server to
	// its certificate: it must chain to the CA bundle of CAFile, or to the
	// system's roots without one, and name the host dialed. CertFile and
	// KeyFile are the client certificate shown to a server that asks for
	// one, given together or not at all. InsecureSkipVerify takes whatever
	// certificate the server shows: the connection is encrypted and nothing
	// tells who is at its far end. Off by default, the connection is
	// plaintext, and the four other keys are read only while it is on.
	TLSEnabled         bool   `json:"tls_enabled" mapstructure:"tls_enabled" ini:"tls_enabled" yaml:"tls_enabled"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enabled bool `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}

func (*MySQL) setDefaults(v *viper.Viper) {
	v.SetDefault("mysql.host", "127.0.0.1")
	v.SetDefault("mysql.port", 3306)
	v.SetDefault("mysql.database", "")
	v.SetDefault("mysql.username", "root")
	v.SetDefault("mysql.password", "")

	v.SetDefault("mysql.dial_timeout", 10*time.Second)
	v.SetDefault("mysql.read_timeout", 0)
	v.SetDefault("mysql.write_timeout", 0)

	v.SetDefault("mysql.strict", true)

	v.SetDefault("mysql.tls_enabled", false)
	v.SetDefault("mysql.cert_file", "")
	v.SetDefault("mysql.key_file", "")
	v.SetDefault("mysql.ca_file", "")
	v.SetDefault("mysql.insecure_skip_verify", false)

	v.SetDefault("mysql.enabled", true)
}
