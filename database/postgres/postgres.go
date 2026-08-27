package postgres

import (
	"database/sql"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	Default *gorm.DB
	db      *sql.DB
)

// Init initializes the default PostgreSQL connection.
// It checks if PostgreSQL is enabled and selected as the default database.
// If the connection is successful, it initializes the database and returns nil.
func Init() (err error) {
	cfg := config.App.Postgres
	if !cfg.Enabled || config.App.Database.Type != config.DBPostgres {
		return nil
	}

	if Default, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to postgres")
	}
	if db, err = Default.DB(); err != nil {
		return errors.Wrap(err, "failed to get postgres db")
	}
	db.SetMaxIdleConns(config.App.Database.MaxIdleConns)
	db.SetMaxOpenConns(config.App.Database.MaxOpenConns)
	db.SetConnMaxLifetime(config.App.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.App.Database.ConnMaxIdleTime)

	zap.S().Infow("successfully connect to postgres", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database, "sslmode", cfg.SSLMode, "timezone", cfg.TimeZone)
	return dbruntime.InitDatabase(Default)
}

// New creates and returns a new PostgreSQL database connection with the given configuration.
// The returned handle already carries the GORM OpenTelemetry tracing plugin,
// so application-held instances passed to DatabaseOn, AggregateOn, and
// TransactionOn are traced like the default database.
func New(cfg config.Postgres) (*gorm.DB, error) {
	// PrepareStmt stays off on purpose: pgx already caches prepared
	// statements at the driver level (QueryExecModeCacheStatement is its
	// default), so the gorm-level cache would only stack a second statement
	// registry on top of it.
	db, err := gorm.Open(postgres.Open(buildDSN(cfg)), &gorm.Config{Logger: logger.Gorm, TranslateError: true, NowFunc: dbruntime.NowUTC})
	if err != nil {
		return nil, err
	}
	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		zap.S().Warnw("failed to install GORM OpenTelemetry tracing plugin", "dialect", "postgres", "error", err)
	}
	return attachReplicas(db, cfg)
}

// attachReplicas wires the configured read replicas into the handle and pins
// its default route to the primary; without replicas the handle is returned
// untouched. Same contract as the MySQL twin: replicas share every DSN
// setting with the primary and differ by address only, the Write pin keeps
// reads on the primary by default, and reads move only through PreferReplica
// models or WithReplica call sites. See WithReplica for the routing
// precedence.
func attachReplicas(db *gorm.DB, cfg config.Postgres) (*gorm.DB, error) {
	if len(cfg.Replicas) == 0 {
		return db, nil
	}
	dialectors := make([]gorm.Dialector, 0, len(cfg.Replicas))
	for _, endpoint := range cfg.Replicas {
		host, port, err := dbruntime.ParseReplicaEndpoint(endpoint)
		if err != nil {
			return nil, errors.Wrap(err, "postgres replicas")
		}
		replicaCfg := cfg
		replicaCfg.Host, replicaCfg.Port = host, port
		replicaCfg.Replicas = nil
		dialectors = append(dialectors, postgres.Open(buildDSN(replicaCfg)))
	}
	return dbruntime.AttachResolver(db, dialectors)
}

func buildDSN(cfg config.Postgres) string {
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
		cfg.Host, cfg.Username, cfg.Password, cfg.Database, cfg.Port, cfg.SSLMode, cfg.TimeZone,
	)
}
