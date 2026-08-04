package clickhouse

import (
	"database/sql"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"go.uber.org/zap"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

var (
	Default *gorm.DB
	db      *sql.DB
)

// Init initializes the default Clickhouse connection.
// It checks if Clickhouse is enabled and selected as the default database.
// If the connection is successful, it initializes the database and returns nil.
func Init() (err error) {
	cfg := config.App.Clickhouse
	if !cfg.Enabled || config.App.Database.Type != config.DBClickHouse {
		return nil
	}

	if Default, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to clickhouse")
	}
	if db, err = Default.DB(); err != nil {
		return errors.Wrap(err, "failed to get clickhouse db")
	}
	db.SetMaxIdleConns(config.App.Database.MaxIdleConns)
	db.SetMaxOpenConns(config.App.Database.MaxOpenConns)
	db.SetConnMaxLifetime(config.App.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.App.Database.ConnMaxIdleTime)

	zap.S().Infow("successfully connect to clickhouse", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database)
	return dbruntime.InitDatabase(Default)
}

// New creates and returns a new Clickhouse database connection with the given configuration.
// The returned handle already carries the GORM OpenTelemetry tracing plugin,
// so application-held instances passed to DatabaseOn and AggregateOn are
// traced like the default database.
//
// ClickHouse is an analytical instance. The handle carries the read side of
// the framework — List/Get/Count/First/Last/Take, the filter operators
// (correlated EXISTS subqueries and JSON containment excepted, those fail
// closed), cursor pagination, and the whole aggregate path including time
// buckets — plus a write path with a deliberately weaker contract: no model
// hooks and no transaction boundary; Create is plain batch INSERTs, Delete a
// lightweight physical DELETE by primary key, Update an asynchronous ALTER
// TABLE mutation for low-frequency data correction. Upsert, Cleanup, the
// transaction boundary, and row locks are not carried and fail with
// database.ErrUnsupportedOnDialect. The schema (engine, ORDER BY,
// partitioning) is hand-written DDL owned by the application; the framework
// never creates or migrates ClickHouse tables.
func New(cfg config.Clickhouse) (*gorm.DB, error) {
	db, err := gorm.Open(clickhouse.Open(buildDSN(cfg)), &gorm.Config{Logger: logger.Gorm, TranslateError: true})
	if err != nil {
		return nil, err
	}
	// It will fix error: "Cannot create column with type 'FixedString(10240)' because fixed string with size > 256 is suspicious. Set setting allow_suspicious_fixed_string_types = 1 in order to allow it"
	sqlDB, err := db.DB()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get clickhouse db")
	}
	if _, err = sqlDB.Exec("SET allow_suspicious_fixed_string_types = 1"); err != nil {
		return nil, err
	}
	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		zap.S().Warnw("failed to install GORM OpenTelemetry tracing plugin", "dialect", "clickhouse", "error", err)
	}
	return db, nil
}

// buildDSN assembles the clickhouse-go DSN, e.g.
// "clickhouse://default:secret@localhost:9000/default?debug=false&compress=false&read_timeout=30s&dial_timeout=5s".
// The query parameters join with "&": an earlier spelling joined them with
// "?", which URL parsing reads as one malformed first parameter and silently
// drops every option after the first. Only options clickhouse-go v2 defines
// may appear — it forwards unknown ones to the server as settings, and the
// server rejects the connection over them.
func buildDSN(cfg config.Clickhouse) string {
	return fmt.Sprintf(
		"clickhouse://%s:%s@%s:%d/%s?debug=%t&compress=%t&read_timeout=%s&dial_timeout=%s",
		cfg.Username, cfg.Password,
		cfg.Host, cfg.Port, cfg.Database,
		cfg.Debug, cfg.Compress, cfg.ReadTimeout, cfg.DialTimeout,
	)
}
