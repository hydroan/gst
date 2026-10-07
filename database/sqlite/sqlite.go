package sqlite

import (
	"database/sql/driver"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var Default *gorm.DB

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

	zap.S().Infow("successfully connect to sqlite", "file", cfg.Path, "is_memory", cfg.IsMemory)
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
// framework's REGEXP implementation and binds every time in UTC; see
// registerRegexpFunc and utcConn. Every handle on the in-memory database
// shares the one database of the process, which lives until the process
// ends; see anchorMemoryDatabase.
// Built without cgo, the driver is a stub whose connections all fail to open,
// so New fails with the driver's error saying the binary needs cgo.
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
	// TranslateError maps the write failures to the gorm sentinels; the
	// framework's own table (see translate) runs over what the driver's
	// leaves as it is.
	db, err := gorm.Open(sqlite.New(sqlite.Config{DriverName: driverName, DSN: dsn}), &gorm.Config{Logger: logger.Gorm, TranslateError: true, NowFunc: dbruntime.NowUTC})
	if err != nil {
		return nil, err
	}
	if err = dbruntime.InstallErrorTranslation(db, translate); err != nil {
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
// process has exactly one in-memory database. Foreign keys are checked on
// its connections as on the file database's (see buildDSN): sqlite checks
// them per connection and only when asked. Times read back in UTC, as they
// are stored (see utcConn).
const memoryDSN = "file::memory:?cache=shared&_foreign_keys=ON&_loc=UTC"

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
// verb clause (see the database package's comment.go), so with that builder
// in place INSERT statements on this dialect alone would go out without their
// trace comment while SELECT, UPDATE and DELETE carry it. Dropping the
// driver's builder hands the clause back to the default rendering, which
// resolves the table through the same placeholder the other dialects use and
// handles the modifier the same way.
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

// buildDSN renders the connection string of the configured database: the
// in-memory database when cfg selects it (see dbruntime.SQLiteInMemory), the
// file at the configured path otherwise.
//
// A path carrying parameters of its own keeps them: the framework's own are
// appended to those, so the connection string never ends up with two question
// marks — sqlite reads the second one as part of a parameter value and the
// tuning is silently lost.
func buildDSN(cfg config.Sqlite) string {
	if dbruntime.SQLiteInMemory(cfg) {
		if len(cfg.Path) == 0 {
			zap.S().Warn("sqlite path is empty, using in-memory database")
		}
		return memoryDSN
	}

	// Add comprehensive SQLite optimization parameters
	params := []string{
		"_journal_mode=WAL",   // Enable WAL mode for better concurrency
		"_busy_timeout=5000",  // 5 second timeout for lock contention
		"_synchronous=NORMAL", // Safe and performant in WAL mode
		"_temp_store=MEMORY",  // Use memory for temporary storage
		"_cache_size=-32000",  // 32MB cache size (negative value means KB)
		"_foreign_keys=ON",    // Enable foreign key constraint checking
		"_loc=UTC",            // Read times back in UTC, as they are stored (see utcConn)
	}

	separator := "?"
	if strings.Contains(cfg.Path, "?") {
		separator = "&"
	}
	return cfg.Path + separator + strings.Join(params, "&")
}
