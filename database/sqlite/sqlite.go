package sqlite

import (
	"database/sql"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database/helper"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	Default *gorm.DB
	db      *sql.DB
	dbmap   = make(map[string]*gorm.DB)
)

// Init initializes the default SQLite connection.
// It checks if SQLite is enabled and selected as the default database.
// If the connection is successful, it initializes the database and returns nil.
func Init() (err error) {
	cfg := config.App.Sqlite
	if !cfg.Enable || config.App.Database.Type != config.DBSqlite {
		return err
	}

	if Default, err = New(cfg); err != nil {
		return errors.Wrap(err, "failed to connect to sqlite")
	}
	if db, err = Default.DB(); err != nil {
		return errors.Wrap(err, "failed to get sqlite db")
	}

	// SQLite works best with limited concurrent connections to avoid lock contention
	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(1) // Use single connection to avoid "database table is locked" errors
	db.SetConnMaxLifetime(config.App.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.App.Database.ConnMaxIdleTime)

	// Optimize database performance with PRAGMA settings
	if err = optimizeDatabase(Default); err != nil {
		zap.S().Warnw("failed to optimize sqlite database", "error", err)
	}

	zap.S().Infow("successfully connect to sqlite", "path", cfg.Path, "database", cfg.Database, "is_memory", cfg.IsMemory)
	return helper.InitDatabase(Default, dbmap)
}

// New creates and returns a new SQLite database connection with the given configuration.
// Returns (*gorm.DB, error) where error is non-nil if the connection fails.
func New(cfg config.Sqlite) (*gorm.DB, error) {
	return gorm.Open(sqlite.Open(buildDSN(cfg)), &gorm.Config{Logger: logger.Gorm})
}

// optimizeDatabase applies performance optimization settings to the SQLite database.
// This function executes PRAGMA optimize to collect statistics and improve query planning.
func optimizeDatabase(db *gorm.DB) error {
	// Execute PRAGMA optimize to collect statistics for better query planning
	if err := db.Exec("PRAGMA optimize").Error; err != nil {
		return errors.Wrap(err, "failed to execute PRAGMA optimize")
	}

	zap.S().Debug("sqlite database optimization completed")
	return nil
}

func buildDSN(cfg config.Sqlite) string {
	dsn := cfg.Path
	if cfg.IsMemory || len(cfg.Path) == 0 {
		if len(cfg.Path) == 0 {
			zap.S().Warn("sqlite path is empty, using in-memory database")
		}
		dsn = "file::memory:?cache=shared" // Ignore file based database if IsMemory is true.
	} else {
		// Add comprehensive SQLite optimization parameters
		params := []string{
			"_journal_mode=WAL",   // Enable WAL mode for better concurrency
			"_busy_timeout=5000",  // 5 second timeout for lock contention
			"_synchronous=NORMAL", // Safe and performant in WAL mode
			"_temp_store=MEMORY",  // Use memory for temporary storage
			"_cache_size=-32000",  // 32MB cache size (negative value means KB)
			"_foreign_keys=ON",    // Enable foreign key constraint checking
		}

		dsn = dsn + "?" + strings.Join(params, "&")
	}
	return dsn
}

// Transaction runs fn in a transaction on the default SQLite connection.
func Transaction(fn func(tx *gorm.DB) error) error { return helper.Transaction(Default, fn) }

// Exec executes raw SQL on the default SQLite connection without returning rows.
func Exec(sql string, values any) error { return helper.Exec(Default, sql, values) }
