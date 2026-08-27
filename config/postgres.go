package config

import "github.com/spf13/viper"

const (
	POSTGRES_HOST     = "POSTGRES_HOST"     //nolint:staticcheck
	POSTGRES_PORT     = "POSTGRES_PORT"     //nolint:staticcheck
	POSTGRES_DATABASE = "POSTGRES_DATABASE" //nolint:staticcheck
	POSTGRES_USERNAME = "POSTGRES_USERNAME" //nolint:staticcheck
	POSTGRES_PASSWORD = "POSTGRES_PASSWORD" //nolint:staticcheck,gosec
	POSTGRES_SSLMODE  = "POSTGRES_SSLMODE"  //nolint:staticcheck
	POSTGRES_TIMEZONE = "POSTGRES_TIMEZONE" //nolint:staticcheck
	POSTGRES_REPLICAS = "POSTGRES_REPLICAS" //nolint:staticcheck
	POSTGRES_ENABLED  = "POSTGRES_ENABLED"  //nolint:staticcheck
)

type Postgres struct {
	Host     string `json:"host" mapstructure:"host" ini:"host" yaml:"host"`
	Port     uint   `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Database string `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	Username string `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password string `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	SSLMode  string `json:"sslmode" mapstructure:"sslmode" ini:"sslmode" yaml:"sslmode"`
	TimeZone string `json:"timezone" mapstructure:"timezone" ini:"timezone" yaml:"timezone"`

	// Replicas lists read-replica endpoints as host:port entries (comma
	// separated in ini), sharing every other connection setting with the
	// primary. Same contract as the MySQL field: replicas make reads
	// eligible for routing, moved only by PreferReplica models or WithReplica
	// call sites; replication, health, and failover belong to the
	// infrastructure.
	Replicas []string `json:"replicas" mapstructure:"replicas" ini:"replicas" yaml:"replicas"`

	Enabled bool `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}

func (*Postgres) setDefault(v *viper.Viper) {
	v.SetDefault("postgres.host", "127.0.0.1")
	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.database", "postgres")
	v.SetDefault("postgres.username", "postgres")
	v.SetDefault("postgres.password", "")
	v.SetDefault("postgres.sslmode", "disable")
	v.SetDefault("postgres.timezone", "UTC")
	v.SetDefault("postgres.enabled", true)
}
