package config

import (
	"time"

	"github.com/spf13/viper"
)

const (
	MYSQL_HOST     = "MYSQL_HOST"     //nolint:staticcheck
	MYSQL_PORT     = "MYSQL_PORT"     //nolint:staticcheck
	MYSQL_DATABASE = "MYSQL_DATABASE" //nolint:staticcheck
	MYSQL_USERNAME = "MYSQL_USERNAME" //nolint:staticcheck
	MYSQL_PASSWORD = "MYSQL_PASSWORD" //nolint:staticcheck

	MYSQL_DIAL_TIMEOUT  = "MYSQL_DIAL_TIMEOUT"  //nolint:staticcheck
	MYSQL_READ_TIMEOUT  = "MYSQL_READ_TIMEOUT"  //nolint:staticcheck
	MYSQL_WRITE_TIMEOUT = "MYSQL_WRITE_TIMEOUT" //nolint:staticcheck

	MYSQL_REPLICAS = "MYSQL_REPLICAS" //nolint:staticcheck

	MYSQL_ENABLED = "MYSQL_ENABLED" //nolint:staticcheck
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
	// database, timeouts, the utf8mb4 charset, the UTC wire location — is
	// shared with the primary, so a replica differs by address only. Configuring replicas
	// makes framework reads eligible for replica routing, but never moves
	// them by default: reads go to a replica only where a model declares
	// PreferReplica or a call site opts in with WithReplica. Replication
	// itself, replica health, and failover belong to the infrastructure;
	// prefer a single load-balanced read endpoint where one exists.
	Replicas []string `json:"replicas" mapstructure:"replicas" ini:"replicas" yaml:"replicas"`

	Enabled bool `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}

func (*MySQL) setDefault(v *viper.Viper) {
	v.SetDefault("mysql.host", "127.0.0.1")
	v.SetDefault("mysql.port", 3306)
	v.SetDefault("mysql.database", "")
	v.SetDefault("mysql.username", "root")
	v.SetDefault("mysql.password", "")

	v.SetDefault("mysql.dial_timeout", 10*time.Second)
	v.SetDefault("mysql.read_timeout", 0)
	v.SetDefault("mysql.write_timeout", 0)

	v.SetDefault("mysql.enabled", true)
}
