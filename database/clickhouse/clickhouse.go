package clickhouse

import (
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

// Init opens the default ClickHouse connection when ClickHouse is enabled and
// selected as the default database, and makes it the default database that
// database.Database and database.DB use; otherwise it does nothing.
func Init() error {
	cfg := config.App.Clickhouse
	if !cfg.Enabled || config.App.Database.Type != config.DBClickHouse {
		return nil
	}

	db, err := New(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to connect to clickhouse")
	}
	zap.S().Infow("successfully connect to clickhouse", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database)
	return dbruntime.InitDatabase(db)
}

// New creates and returns a new ClickHouse database connection with the given
// configuration. With tracing configured on, the returned handle carries the
// GORM OpenTelemetry tracing plugin, so application-held instances passed to
// DatabaseOn, SelectOn and UnionAllOn are traced like the default database. The
// pool runs under the connection limits of the [database] configuration, the
// same ones the default handle runs under.
//
// ClickHouse is an analytical instance. The handle carries the read side of
// the framework — List/Get/Count/First/Last/Take, the filter operators
// (correlated EXISTS subqueries and JSON containment excepted, those fail
// closed), cursor pagination, and the whole select path: grouping, time
// buckets, window functions, UnionAll and the joined selects of Select —
// plus a write path with a deliberately weaker contract: no model hooks and
// no transaction boundary; Create is plain batch INSERTs, Delete a
// lightweight physical DELETE by primary key, Update an asynchronous ALTER
// TABLE mutation for low-frequency data correction. Upsert, Cleanup, a
// delete by conditions, the transaction boundary, row locks, and the model
// joins of Select (no unique constraint to prove a join on) are not carried
// and fail with database.ErrUnsupportedOnDialect; the database package doc
// lists the whole contract. The schema (engine, ORDER BY, partitioning) is
// hand-written DDL owned by the application; the framework never creates or
// migrates ClickHouse tables.
func New(cfg config.Clickhouse) (*gorm.DB, error) {
	// PrepareStmt stays off on purpose: clickhouse-go implements Prepare as
	// its batch-INSERT mechanism only, so routing ordinary statements through
	// a statement cache would break every SELECT.
	db, err := gorm.Open(clickhouse.Open(buildDSN(cfg)), &gorm.Config{Logger: logger.Gorm, TranslateError: true, NowFunc: dbruntime.NowUTC})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get clickhouse db")
	}
	dbruntime.ConfigurePool(sqlDB)
	dbruntime.InstallTracing(db)
	return db, nil
}

// buildDSN assembles the clickhouse-go DSN, for example:
//
//	clickhouse://default:secret@localhost:9000/default?debug=false&compress=false&read_timeout=30s&dial_timeout=5s&join_use_nulls=1
//
// The query parameters join with "&": joined with "?", URL parsing reads them
// as one malformed first parameter and silently drops every option after the
// first. Only options clickhouse-go v2 defines and server settings may appear —
// it forwards unknown ones to the server as settings, and the server rejects
// the connection over a setting it does not know.
//
// join_use_nulls is the one server setting set here. ClickHouse fills the
// unmatched side of a LEFT JOIN with the columns' default values unless it
// is on; the framework's joins promise NULL there, the same answer every
// other dialect gives and the one a pointer result field can tell from a
// real zero, so the setting travels with every connection of the instance.
func buildDSN(cfg config.Clickhouse) string {
	return fmt.Sprintf(
		"clickhouse://%s:%s@%s:%d/%s?debug=%t&compress=%t&read_timeout=%s&dial_timeout=%s&join_use_nulls=1",
		cfg.Username, cfg.Password,
		cfg.Host, cfg.Port, cfg.Database,
		cfg.Debug, cfg.Compress, cfg.ReadTimeout, cfg.DialTimeout,
	)
}
