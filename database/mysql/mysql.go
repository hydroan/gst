package mysql

import (
	"database/sql"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	Default *gorm.DB
	db      *sql.DB
	dbmap   = make(map[string]*gorm.DB)
)

// Init initializes the default MySQL connection.
// It checks if MySQL is enabled and selected as the default database.
// If the connection is successful, it initializes the database and returns nil.
func Init() (err error) {
	cfg := config.App.MySQL
	if !cfg.Enabled || config.App.Database.Type != config.DBMySQL {
		return err
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
	return dbruntime.InitDatabase(Default, dbmap)
}

// New creates and returns a new MySQL database connection with the given configuration.
// Returns (*gorm.DB, error) where error is non-nil if the connection fails.
func New(cfg config.MySQL) (*gorm.DB, error) {
	// TranslateError maps dialect-specific write failures to portable gorm
	// sentinels (gorm.ErrDuplicatedKey, gorm.ErrForeignKeyViolated) that
	// database.Create/Update surface to callers.
	return gorm.Open(mysql.Open(buildDSN(cfg)), &gorm.Config{Logger: logger.Gorm, TranslateError: true})
}

// buildDSN assembles the go-sql-driver DSN. clientFoundRows=true makes UPDATE
// report matched rows instead of changed rows — the SQL-standard semantics
// every other supported dialect already uses. database.Update depends on it:
// with changed-rows semantics, saving a record without modifying anything
// reports zero affected rows and would be misread as ErrRecordNotFound.
func buildDSN(cfg config.MySQL) string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local&clientFoundRows=true",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.Charset,
	)
}
