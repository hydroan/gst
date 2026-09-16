package sqlite

import (
	"database/sql"
	"database/sql/driver"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	sqlite3 "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var Default *gorm.DB

// driverName is the database/sql driver this package opens connections with:
// the stock sqlite3 driver extended with the SQL functions the framework's
// query surface needs. Registered at package load, which is how database/sql
// drivers are installed.
const driverName = "gst_sqlite3"

// sqliteDriver is the driver registered under driverName.
var sqliteDriver = &sqlite3.SQLiteDriver{
	ConnectHook: registerRegexpFunc,
}

func init() {
	sql.Register(driverName, sqliteDriver)
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
	// Optimize database performance with PRAGMA settings
	if err = optimizeDatabase(Default); err != nil {
		zap.S().Warnw("failed to optimize sqlite database", "error", err)
	}

	zap.S().Infow("successfully connect to sqlite", "path", cfg.Path, "database", cfg.Database, "is_memory", cfg.IsMemory)
	return dbruntime.InitDatabase(Default)
}

// New creates and returns a new SQLite database connection with the given configuration.
// With tracing configured on, the returned handle carries the GORM
// OpenTelemetry tracing plugin, so application-held instances passed to
// DatabaseOn, SelectOn, UnionAllOn, TransactionOn, and CleanupOn are traced
// like the default database.
// The pool runs under the connection limits of the [database] configuration
// narrowed to a single connection, the same as the default handle.
// Connections open through this package's own driver, which carries the
// framework's REGEXP implementation; see registerRegexpFunc. Every handle on
// the in-memory database shares the one database of the process, which lives
// until the process ends; see anchorMemoryDatabase.
func New(cfg config.Sqlite) (*gorm.DB, error) {
	dsn := buildDSN(cfg)
	if dsn == memoryDSN {
		if err := anchorMemoryDatabase(); err != nil {
			return nil, err
		}
	}
	// No PrepareStmt: the framework's SQL comments carry the request's trace
	// id (see the database package's comment.go), making statement texts
	// request-unique, so a text-keyed statement cache would hold one dead
	// entry per request. Sqlite compiles statements in-process at
	// microsecond cost, so each run simply compiles its statement.
	db, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: driverName, DSN: dsn}), &gorm.Config{Logger: logger.Gorm, TranslateError: true, NowFunc: dbruntime.NowUTC})
	if err != nil {
		return nil, err
	}
	restoreInsertClauseContract(db)
	pool, err := db.DB()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sqlite db")
	}
	dbruntime.ConfigurePool(pool)
	// SQLite admits one writer at a time, and connections sharing the
	// in-memory database's cache lock whole tables against each other: a
	// pool of one connection makes statements queue for it instead of
	// running into "database is locked" and "database table is locked". The
	// lifetime and idle-time limits stay as configured, since the in-memory
	// database outlives the pool's connection; see anchorMemoryDatabase.
	pool.SetMaxIdleConns(1)
	pool.SetMaxOpenConns(1)
	dbruntime.InstallTracing(db)
	return db, nil
}

// memoryDSN names the in-memory database. Connections sharing its cache
// share one database, and the name is the same everywhere in a process, so a
// process has exactly one in-memory database.
const memoryDSN = "file::memory:?cache=shared"

var (
	// memoryAnchorMu guards memoryAnchor.
	memoryAnchorMu sync.Mutex
	// memoryAnchor is the connection keeping the in-memory database alive,
	// see anchorMemoryDatabase. It stays referenced from here on purpose:
	// the driver closes a connection nothing references any more.
	memoryAnchor driver.Conn
)

// anchorMemoryDatabase opens a connection to the in-memory database, once per
// process, and holds it until the process ends, so the database lives exactly
// as long as the process.
//
// SQLite deletes an in-memory database the moment its last connection
// closes, and a pool closes its connection on its own: past the configured
// lifetime or idle time, and after a transaction whose context ended before
// it finished — database/sql discards that connection instead of reusing it,
// because this driver cannot reset a session. The tables are created as the
// process starts, so with the pool's connection the only one, every table
// and row would go with it. The anchor runs no statement once open, so it
// never holds a table lock the pool's connection could wait on.
func anchorMemoryDatabase() error {
	memoryAnchorMu.Lock()
	defer memoryAnchorMu.Unlock()

	if memoryAnchor != nil {
		return nil
	}
	conn, err := sqliteDriver.Open(memoryDSN)
	if err != nil {
		return errors.Wrap(err, "failed to open the in-memory sqlite database")
	}
	memoryAnchor = conn
	return nil
}

// restoreInsertClauseContract renders INSERT the way every other dialect does.
// The sqlite driver installs its own INSERT clause builder, which writes the
// verb, the modifier and the table and returns, skipping the before- and
// after-expressions gorm's default clause rendering places around them. The
// framework registers the statement comment as the after-expression of every
// verb clause (see the database package's comment.go), so on this dialect
// alone INSERT statements went out without their trace comment while SELECT,
// UPDATE and DELETE carried it. Dropping the driver's builder hands the clause
// back to the default rendering, which resolves the table through the same
// placeholder the other dialects use and handles the modifier the same way.
func restoreInsertClauseContract(db *gorm.DB) {
	delete(db.ClauseBuilders, clause.Insert{}.Name())
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
		dsn = memoryDSN // Ignore file based database if IsMemory is true.
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
