package sqlite

import (
	"database/sql"
	"regexp"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	Default *gorm.DB
	db      *sql.DB
)

// driverName is the database/sql driver this package opens connections with:
// the stock sqlite3 driver extended with the SQL functions the framework's
// query surface needs. Registered at package load, which is how database/sql
// drivers are installed.
const driverName = "gst_sqlite3"

func init() {
	sql.Register(driverName, &sqlite3.SQLiteDriver{
		ConnectHook: registerRegexpFunc,
	})
}

// registerRegexpFunc makes REGEXP work on conn. SQLite parses the operator
// but ships no implementation: "value REGEXP pattern" invokes a user function
// regexp(pattern, value), and without one every regex filter fails at runtime
// with "no such function: REGEXP".
//
// The implementation is Go's regexp package, so patterns use RE2 syntax and
// match case-sensitively like the PostgreSQL ~ operator; MySQL matches
// case-insensitively only through its default collation, which is a collation
// choice rather than framework behavior. (?i) opts into case-insensitivity
// per pattern, and an invalid pattern fails the query the way every dialect
// rejects one.
//
// A NULL value or pattern never matches, mirroring how MySQL answers NULL
// with NULL and a WHERE treats that as false. Numeric values match against
// their text form the way MySQL casts them.
//
// The compiled pattern is cached per connection: the rows of one statement
// all carry the same pattern, and a connection runs statements one at a time,
// so the single-entry cache needs no lock.
func registerRegexpFunc(conn *sqlite3.SQLiteConn) error {
	var lastPattern string
	var lastRe *regexp.Regexp
	return conn.RegisterFunc("regexp", func(patternArg, valueArg any) (bool, error) {
		pattern, ok := textValue(patternArg)
		if !ok {
			return false, nil
		}
		value, ok := textValue(valueArg)
		if !ok {
			return false, nil
		}
		if lastRe == nil || pattern != lastPattern {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false, err
			}
			lastPattern, lastRe = pattern, re
		}
		return lastRe.MatchString(value), nil
	}, true)
}

// textValue renders a SQLite value as the text REGEXP matches against.
// NULL and BLOB report false: neither has a text form a pattern should
// silently match.
func textValue(arg any) (string, bool) {
	switch v := arg.(type) {
	case string:
		return v, true
	case int64:
		return strconv.FormatInt(v, 10), true
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), true
	default:
		return "", false
	}
}

// Init initializes the default SQLite connection.
// It checks if SQLite is enabled and selected as the default database.
// If the connection is successful, it initializes the database and returns nil.
func Init() (err error) {
	cfg := config.App.Sqlite
	if !cfg.Enabled || config.App.Database.Type != config.DBSqlite {
		return nil
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
	return dbruntime.InitDatabase(Default)
}

// New creates and returns a new SQLite database connection with the given configuration.
// The returned handle already carries the GORM OpenTelemetry tracing plugin,
// so application-held instances passed to DatabaseOn, AggregateOn, and
// TransactionOn are traced like the default database.
// Connections open through this package's own driver, which carries the
// framework's REGEXP implementation; see registerRegexpFunc.
func New(cfg config.Sqlite) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: driverName, DSN: buildDSN(cfg)}), &gorm.Config{Logger: logger.Gorm, TranslateError: true, NowFunc: dbruntime.NowUTC})
	if err != nil {
		return nil, err
	}
	if err := db.Use(otelgorm.NewPlugin()); err != nil {
		zap.S().Warnw("failed to install GORM OpenTelemetry tracing plugin", "dialect", "sqlite", "error", err)
	}
	return db, nil
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
