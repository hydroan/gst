package clickhouse

import (
	"database/sql"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database/helper"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

var (
	Default *gorm.DB
	db      *sql.DB
	dbmap   = make(map[string]*gorm.DB)
)

// Init initializes the default Clickhouse connection.
// It checks if Clickhouse is enabled and selected as the default database.
// If the connection is successful, it initializes the database and returns nil.
func Init() (err error) {
	cfg := config.App.Clickhouse
	if !cfg.Enable || config.App.Database.Type != config.DBClickHouse {
		return err
	}

	if Default, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to clickhouse")
	}
	if db, err = Default.DB(); err != nil {
		return errors.Wrap(err, "failed to get clickhouse db")
	}
	// It will fix error: "Cannot create column with type 'FixedString(10240)' because fixed string with size > 256 is suspicious. Set setting allow_suspicious_fixed_string_types = 1 in order to allow it"
	if _, err = db.Exec("SET allow_suspicious_fixed_string_types = 1"); err != nil {
		return err
	}
	db.SetMaxIdleConns(config.App.Database.MaxIdleConns)
	db.SetMaxOpenConns(config.App.Database.MaxOpenConns)
	db.SetConnMaxLifetime(config.App.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.App.Database.ConnMaxIdleTime)

	zap.S().Infow("successfully connect to clickhouse", "host", cfg.Host, "port", cfg.Port, "database", cfg.Database)
	return helper.InitDatabase(Default, dbmap)
}

// New creates and returns a new Clickhouse database connection with the given configuration.
// Returns (*gorm.DB, error) where error is non-nil if the connection fails.
func New(cfg config.Clickhouse) (*gorm.DB, error) {
	return gorm.Open(clickhouse.Open(buildDSN(cfg)), &gorm.Config{Logger: logger.Gorm})
}

func buildDSN(cfg config.Clickhouse) string {
	// return "clickhouse://default:clickhouse@localhost:9010/default?debug=true?compress=false?read_timeout=5s?write_timeout=5s?dial_timeout=5s"
	return fmt.Sprintf(
		"clickhouse://%s:%s@%s:%d/%s?debug=%t?compress=%t?read_timeout=%s?write_timeout=%s?dial_timeout=%s",
		cfg.Username, cfg.Password,
		cfg.Host, cfg.Port, cfg.Database,
		cfg.Debug, cfg.Compress, cfg.ReadTimeout, cfg.WriteTimeout, cfg.DialTimeout,
	)
}
