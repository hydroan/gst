package sqlserver

import (
	"database/sql"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"go.uber.org/zap"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

var (
	Default *gorm.DB
	db      *sql.DB
)

// Init initializes the default SQLServer connection.
// It checks if SQLServer is enabled and selected as the default database.
// If the connection is successful, it initializes the database and returns nil.
func Init() (err error) {
	cfg := config.App.SQLServer
	if !cfg.Enabled || config.App.Database.Type != config.DBSQLServer {
		return nil
	}

	if Default, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to sqlserver")
	}
	if db, err = Default.DB(); err != nil {
		return errors.Wrap(err, "failed to get sqlserver db")
	}
	db.SetMaxIdleConns(config.App.Database.MaxIdleConns)
	db.SetMaxOpenConns(config.App.Database.MaxOpenConns)
	db.SetConnMaxLifetime(config.App.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.App.Database.ConnMaxIdleTime)

	zap.S().Infow("successfully connect to sqlserver", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database)
	return dbruntime.InitDatabase(Default)
}

// New creates and returns a new SQLServer database connection with the given configuration.
// The returned handle already carries the GORM OpenTelemetry tracing plugin,
// so application-held instances passed to DatabaseOn, AggregateOn, and
// TransactionOn are traced like the default database.
func New(cfg config.SQLServer) (*gorm.DB, error) {
	// PrepareStmt caches prepared statements (sp_prepare/sp_execute on this
	// dialect), so a statement is parsed and planned once per connection.
	db, err := gorm.Open(sqlserver.Open(buildDSN(cfg)), &gorm.Config{Logger: logger.Gorm, TranslateError: true, NowFunc: dbruntime.NowUTC, PrepareStmt: true})
	if err != nil {
		return nil, err
	}
	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		zap.S().Warnw("failed to install GORM OpenTelemetry tracing plugin", "dialect", "sqlserver", "error", err)
	}
	return db, nil
}

func buildDSN(cfg config.SQLServer) string {
	return fmt.Sprintf(
		"sqlserver://%s:%s@%s:%d?database=%s&encrypt=%v&trustServerCertificate=%v",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
		cfg.Encrypt, cfg.TrustServer,
	)
}
