package mysql

import (
	"database/sql"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	Default *gorm.DB
	db      *sql.DB
)

// Init initializes the default MySQL connection.
// It checks if MySQL is enabled and selected as the default database.
// If the connection is successful, it initializes the database and returns nil.
func Init() (err error) {
	cfg := config.App.MySQL
	if !cfg.Enabled || config.App.Database.Type != config.DBMySQL {
		return nil
	}

	if Default, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to mysql")
	}
	if db, err = Default.DB(); err != nil {
		return errors.Wrap(err, "failed to get mysql db")
	}
	db.SetMaxIdleConns(config.App.Database.MaxIdleConns)
	db.SetMaxOpenConns(config.App.Database.MaxOpenConns)
	db.SetConnMaxLifetime(config.App.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.App.Database.ConnMaxIdleTime)

	zap.S().Infow("successfully connect to mysql", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database)
	return dbruntime.InitDatabase(Default)
}

// New creates and returns a new MySQL database connection with the given configuration.
// The returned handle already carries the GORM OpenTelemetry tracing plugin,
// so application-held instances passed to DatabaseOn, AggregateOn, and
// TransactionOn are traced like the default database.
func New(cfg config.MySQL) (*gorm.DB, error) {
	// TranslateError maps dialect-specific write failures to portable gorm
	// sentinels (gorm.ErrDuplicatedKey, gorm.ErrForeignKeyViolated) that
	// database.Create/Update surface to callers. Statements run over the
	// text protocol with client-side parameter interpolation — no gorm
	// PrepareStmt, no server-side prepared statements; buildDSN explains
	// why.
	db, err := gorm.Open(mysql.Open(buildDSN(cfg)), &gorm.Config{Logger: logger.Gorm, TranslateError: true, NowFunc: dbruntime.NowUTC})
	if err != nil {
		return nil, err
	}
	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		zap.S().Warnw("failed to install GORM OpenTelemetry tracing plugin", "dialect", "mysql", "error", err)
	}
	return attachReplicas(db, cfg)
}

// attachReplicas wires the configured read replicas into the handle and pins
// its default route to the primary. Without replicas it returns the handle
// untouched, so a replica-free deployment pays nothing.
//
// Each replica shares the primary's DSN settings — credentials, database,
// charset, timeouts, and the UTC wire location, all of which reads depend on
// — and differs by address only. The returned handle carries a Write pin:
// with the resolver installed, gorm would otherwise route every plain read
// to a replica, and that silent default breaks read-your-writes everywhere
// (a session read right after login, a list right after create). Reads move
// only where a model declares PreferReplica or a call site opts in with
// WithReplica; transactions never move at all, because the resolver leaves
// statements inside one alone. See WithReplica for the full routing
// precedence.
func attachReplicas(db *gorm.DB, cfg config.MySQL) (*gorm.DB, error) {
	if len(cfg.Replicas) == 0 {
		return db, nil
	}
	dialectors := make([]gorm.Dialector, 0, len(cfg.Replicas))
	for _, endpoint := range cfg.Replicas {
		host, port, err := dbruntime.ParseReplicaEndpoint(endpoint)
		if err != nil {
			return nil, errors.Wrap(err, "mysql replicas")
		}
		replicaCfg := cfg
		replicaCfg.Host, replicaCfg.Port = host, port
		replicaCfg.Replicas = nil
		dialectors = append(dialectors, mysql.Open(buildDSN(replicaCfg)))
	}
	return dbruntime.AttachResolver(db, dialectors)
}

// buildDSN assembles the go-sql-driver DSN. clientFoundRows=true makes UPDATE
// report matched rows instead of changed rows — the SQL-standard semantics
// every other supported dialect already uses. database.Update depends on it:
// with changed-rows semantics, saving a record without modifying anything
// reports zero affected rows and would be misread as ErrRecordNotFound.
//
// interpolateParams=true renders parameters into the statement text on the
// client — one round trip per statement over the text protocol, no
// server-side prepared statements. The framework's SQL comments carry the
// request's trace id (see the database package's comment.go), making every
// statement text request-unique, so a text-keyed statement cache — gorm's
// PrepareStmt above the driver or the server's prepared statements under it
// — would never be reused and only grow: dead client entries, dead
// per-connection server handles counted against the server-global
// max_prepared_stmt_count. Interpolation's escaping is charset-sensitive:
// it is sound under the default utf8mb4, and the unsafe multibyte charsets
// (big5, cp932, gb2312, gbk, sjis) must never be configured — the driver's
// own refusal covers only the collation DSN parameter, not charset, so
// nothing downstream catches a bad charset here.
//
// loc=UTC makes the driver store and read DATETIME values as UTC wall-clock
// time, which is the framework's one time base across dialects: postgres
// already normalizes timestamptz to UTC and sqlite time text is read in UTC
// by its date functions. A local loc would make the same instant a different
// stored wall clock per dialect (and per server timezone), which is what
// breaks time bucket labels, boundary comparisons, and URL time filters.
//
// The timeout parameters mirror config.MySQL: timeout (dial) is on by
// default so a black-holed host fails in seconds instead of blocking until
// the OS gives up; readTimeout and writeTimeout are appended only when
// configured, because their default is deliberately off — see the field
// comments on config.MySQL.
func buildDSN(cfg config.MySQL) string {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=UTC&clientFoundRows=true&interpolateParams=true",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.Charset,
	)
	if cfg.DialTimeout > 0 {
		dsn += "&timeout=" + cfg.DialTimeout.String()
	}
	if cfg.ReadTimeout > 0 {
		dsn += "&readTimeout=" + cfg.ReadTimeout.String()
	}
	if cfg.WriteTimeout > 0 {
		dsn += "&writeTimeout=" + cfg.WriteTimeout.String()
	}
	return dsn
}
