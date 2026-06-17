package config

import "time"

type DBType string

const (
	DBSqlite     DBType = "sqlite"
	DBPostgres   DBType = "postgres"
	DBMySQL      DBType = "mysql"
	DBSQLServer  DBType = "sqlserver"
	DBClickHouse DBType = "clickhouse"
)

const (
	DATABASE_TYPE                 = "DATABASE_TYPE"                 //nolint:staticcheck
	DATABASE_SLOW_QUERY_THRESHOLD = "DATABASE_SLOW_QUERY_THRESHOLD" //nolint:staticcheck
	DATABASE_MAX_IDLE_CONNS       = "DATABASE_MAX_IDLE_CONNS"       //nolint:staticcheck
	DATABASE_MAX_OPEN_CONNS       = "DATABASE_MAX_OPEN_CONNS"       //nolint:staticcheck
	DATABASE_CONN_MAX_LIFETIME    = "DATABASE_CONN_MAX_LIFETIME"    //nolint:staticcheck
	DATABASE_CONN_MAX_IDLE_TIME   = "DATABASE_CONN_MAX_IDLE_TIME"   //nolint:staticcheck
)

type Database struct {
	Type               DBType        `json:"type" mapstructure:"type" ini:"type" yaml:"type"`
	SlowQueryThreshold time.Duration `json:"slow_query_threshold" mapstructure:"slow_query_threshold" ini:"slow_query_threshold" yaml:"slow_query_threshold"`
	MaxIdleConns       int           `json:"max_idle_conns" mapstructure:"max_idle_conns" ini:"max_idle_conns" yaml:"max_idle_conns"`
	MaxOpenConns       int           `json:"max_open_conns" mapstructure:"max_open_conns" ini:"max_open_conns" yaml:"max_open_conns"`
	ConnMaxLifetime    time.Duration `json:"conn_max_lifetime" mapstructure:"conn_max_lifetime" ini:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime    time.Duration `json:"conn_max_idle_time" mapstructure:"conn_max_idle_time" ini:"conn_max_idle_time" yaml:"conn_max_idle_time"`
}

func (*Database) setDefault() {
	cv.SetDefault("database.type", DBSqlite)
	cv.SetDefault("database.slow_query_threshold", 500*time.Millisecond)
	cv.SetDefault("database.max_idle_conns", 100)
	cv.SetDefault("database.max_open_conns", 100)
	cv.SetDefault("database.conn_max_lifetime", 1*time.Hour)
	cv.SetDefault("database.conn_max_idle_time", 10*time.Minute)
}
