package postgres

import (
	"database/sql"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database/helper"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	Default *gorm.DB
	db      *sql.DB
	dbmap   = make(map[string]*gorm.DB)
)

// Init initializes the default PostgreSQL connection.
// It checks if PostgreSQL is enabled and selected as the default database.
// If the connection is successful, it initializes the database and returns nil.
func Init() (err error) {
	cfg := config.App.Postgres
	if !cfg.Enable || config.App.Database.Type != config.DBPostgres {
		return err
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
	return helper.InitDatabase(Default, dbmap)
}

// New creates and returns a new PostgreSQL database connection with the given configuration.
// Returns (*gorm.DB, error) where error is non-nil if the connection fails.
func New(cfg config.Postgres) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(buildDSN(cfg)), &gorm.Config{Logger: logger.Gorm})
}

func buildDSN(cfg config.Postgres) string {
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
		cfg.Host, cfg.Username, cfg.Password, cfg.Database, cfg.Port, cfg.SSLMode, cfg.TimeZone,
	)
}

// Transaction runs fn in a transaction on the default PostgreSQL connection.
func Transaction(fn func(tx *gorm.DB) error) error { return helper.Transaction(Default, fn) }

// Exec executes raw SQL on the default PostgreSQL connection without returning rows.
func Exec(sql string, values any) error { return helper.Exec(Default, sql, values) }
