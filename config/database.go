package config

import (
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/viper"
)

type DBType string

const (
	DBSqlite     DBType = "sqlite"
	DBPostgres   DBType = "postgres"
	DBMySQL      DBType = "mysql"
	DBClickHouse DBType = "clickhouse"
)

const (
	DATABASE_TYPE                 = "DATABASE_TYPE"                 //nolint:staticcheck
	DATABASE_AUTO_MIGRATE         = "DATABASE_AUTO_MIGRATE"         //nolint:staticcheck
	DATABASE_SLOW_QUERY_THRESHOLD = "DATABASE_SLOW_QUERY_THRESHOLD" //nolint:staticcheck
	DATABASE_MAX_IDLE_CONNS       = "DATABASE_MAX_IDLE_CONNS"       //nolint:staticcheck
	DATABASE_MAX_OPEN_CONNS       = "DATABASE_MAX_OPEN_CONNS"       //nolint:staticcheck
	DATABASE_CONN_MAX_LIFETIME    = "DATABASE_CONN_MAX_LIFETIME"    //nolint:staticcheck
	DATABASE_CONN_MAX_IDLE_TIME   = "DATABASE_CONN_MAX_IDLE_TIME"   //nolint:staticcheck
	DATABASE_SQL_COMMENT          = "DATABASE_SQL_COMMENT"          //nolint:staticcheck
)

// SQLCommentMode selects what the framework annotates onto every SQL
// statement as a key='value' comment, for the database-side view:
// SHOW PROCESSLIST, the slow query log, and audit plugins show the comment,
// so an operator holding a problem statement can see where it came from
// without a reverse text search through the application logs.
type SQLCommentMode string

const (
	// SQLCommentOff renders no statement comments.
	SQLCommentOff SQLCommentMode = "off"

	// SQLCommentRoute annotates statements with the issuing HTTP route, e.g.
	// /*route='%2Fapi%2Fv1%2Fusers'*/. The default: routes are a small,
	// fixed set, so the prepared-statement cache splits by at most the
	// handful of routes issuing each statement shape and server-side
	// statement caching keeps its value.
	SQLCommentRoute SQLCommentMode = "route"

	// SQLCommentTrace adds the request's trace id to the route comment,
	// letting an operator jump from a captured statement straight to the
	// request's full trail in the log store. A per-request-unique comment
	// makes every statement text unique, which would defeat server-side
	// statement caching outright — so this mode also switches the
	// connection to per-statement text protocol (MySQL interpolateParams,
	// postgres simple protocol) at initialization. The trade is deliberate:
	// one round trip either way, at the cost of per-statement server-side
	// parsing. Choose it when database-side incident work matters more than
	// that margin.
	SQLCommentTrace SQLCommentMode = "trace"
)

// Validate rejects an unknown mode at startup: a typo silently treated as
// "off" would remove the annotations an operator relies on.
func (m SQLCommentMode) Validate() error {
	switch m {
	case SQLCommentOff, SQLCommentRoute, SQLCommentTrace:
		return nil
	}
	return errors.Newf("invalid database.sql_comment %q: want off, route, or trace", string(m))
}

type Database struct {
	Type               DBType        `json:"type" mapstructure:"type" ini:"type" yaml:"type"`
	AutoMigrate        bool          `json:"auto_migrate" mapstructure:"auto_migrate" ini:"auto_migrate" yaml:"auto_migrate"`
	SlowQueryThreshold time.Duration `json:"slow_query_threshold" mapstructure:"slow_query_threshold" ini:"slow_query_threshold" yaml:"slow_query_threshold"`
	MaxIdleConns       int           `json:"max_idle_conns" mapstructure:"max_idle_conns" ini:"max_idle_conns" yaml:"max_idle_conns"`
	MaxOpenConns       int           `json:"max_open_conns" mapstructure:"max_open_conns" ini:"max_open_conns" yaml:"max_open_conns"`
	ConnMaxLifetime    time.Duration `json:"conn_max_lifetime" mapstructure:"conn_max_lifetime" ini:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	ConnMaxIdleTime    time.Duration `json:"conn_max_idle_time" mapstructure:"conn_max_idle_time" ini:"conn_max_idle_time" yaml:"conn_max_idle_time"`

	// SQLComment selects the statement comment mode; see SQLCommentMode.
	SQLComment SQLCommentMode `json:"sql_comment" mapstructure:"sql_comment" ini:"sql_comment" yaml:"sql_comment"`
}

func (*Database) setDefault(v *viper.Viper) {
	v.SetDefault("database.type", DBSqlite)
	v.SetDefault("database.auto_migrate", false)
	v.SetDefault("database.slow_query_threshold", 500*time.Millisecond)
	v.SetDefault("database.max_idle_conns", 100)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime", 1*time.Hour)
	v.SetDefault("database.conn_max_idle_time", 10*time.Minute)
	v.SetDefault("database.sql_comment", string(SQLCommentRoute))
}
