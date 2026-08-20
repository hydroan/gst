// Package dbmigrate renders registered Go models into a target schema and
// migrates a database towards it.
//
// SchemaDumper produces the target schema from the models themselves, so it
// always matches the DDL the runtime applies. Migrate then diffs that schema
// against the live database through sqldef and applies the difference, or
// only plans it in dry-run mode. A plan that would drop and re-create an
// identical definition comes back with advisory text offering the
// metadata-only rename instead; executing it stays a human decision.
package dbmigrate

import (
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/sqldef/sqldef/v3"
	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/database/mysql"
	"github.com/sqldef/sqldef/v3/database/postgres"
	"github.com/sqldef/sqldef/v3/parser"
	"github.com/sqldef/sqldef/v3/schema"
)

// DatabaseConfig is the connection the migration runs against.
type DatabaseConfig struct {
	// Database is the schema name on MySQL and PostgreSQL, and the file path
	// on SQLite.
	Database string
	Username string
	Password string
	Host     string
	Port     int
	// SSLMode is the PostgreSQL sslmode parameter; the other dialects ignore it.
	SSLMode string
}

// MigrateOption tunes a single migration run.
type MigrateOption struct {
	// DryRun plans the migration and reports what would change without
	// touching the database.
	DryRun bool
	// EnableDrop lets the plan contain destructive statements. Without it
	// sqldef keeps every table, column and index the models no longer declare.
	EnableDrop bool
}

// Migrate applies the schema changes to the database.
// It returns true if any changes were applied (or would be applied in dry-run mode),
// and false if the database schema is already up-to-date.
//
// Index renames must run through this migration path BEFORE deploying code
// that carries the new index name: once the rename is applied, startup table
// preparation matches the new name and does nothing. With database.auto_migrate
// enabled (local development, tests), deploying first instead makes gorm's
// MySQL driver silently DROP and re-CREATE single-column unique indexes during
// startup, which rebuilds the index with a full table scan and skips every
// review step. With auto_migrate disabled (the production default) nothing is
// rebuilt, but the model and the database keep drifting until the migration
// runs.
//
// When a MySQL plan drops and re-creates an identical definition — an index
// on the same table, or a whole table under a new name — the suspected
// renames are returned as advisory text with ready-to-run RENAME INDEX and
// RENAME TABLE guidance. For tables the advisory doubles as a data-loss
// guard, because the planned DROP TABLE would discard every row that the
// metadata-only rename keeps. The caller owns when and how to present it;
// executing the rename stays a human decision.
func Migrate(schemas []string, dbtyp config.DBType, cfg *DatabaseConfig, opt *MigrateOption) (migrated bool, advisory string, err error) {
	if len(schemas) == 0 || cfg == nil {
		return false, "", nil
	}
	if opt == nil {
		opt = &MigrateOption{}
	}

	dbcfg := database.Config{
		DbName:   cfg.Database,
		User:     cfg.Username,
		Password: cfg.Password,
		Host:     cfg.Host,
		Port:     cfg.Port,
		SslMode:  cfg.SSLMode,
	}
	migOpt := &sqldef.Options{
		DryRun:      opt.DryRun,
		DesiredDDLs: strings.Join(schemas, ";\n"),
		Config: database.GeneratorConfig{
			EnableDrop: opt.EnableDrop,
		},
	}

	var db database.Database
	var parseMode parser.ParserMode
	var genMode schema.GeneratorMode

	switch dbtyp {
	case config.DBMySQL:
		db, err = mysql.NewDatabase(dbcfg)
		parseMode = parser.ParserModeMysql
		genMode = schema.GeneratorModeMysql
	case config.DBPostgres:
		db, err = postgres.NewDatabase(dbcfg)
		parseMode = parser.ParserModePostgres
		genMode = schema.GeneratorModePostgres
	case config.DBSqlite:
		db, err = newSQLiteDatabase(dbcfg)
		parseMode = parser.ParserModeSQLite3
		genMode = schema.GeneratorModeSQLite3
	default:
		// ClickHouse (and any other analytical store) is deliberately not
		// migratable here: its schema is a query-model design — engine,
		// ORDER BY, partitioning, TTL — that cannot be derived from Go models,
		// so the application owns it through hand-written DDL.
		return false, "", errors.Newf("schema migration does not support %q: its schema is managed by hand-written DDL on the application side", dbtyp)
	}
	if err != nil {
		return false, "", err
	}
	defer db.Close()

	sqlParser := database.NewParser(parseMode)
	return runMigration(genMode, db, sqlParser, migOpt)
}

// runMigration executes the database migration logic.
// This function is derived from sqldef.Run (https://github.com/sqldef/sqldef),
// but modified to return a boolean indicating whether any migration was
// performed, the rename advisory text for the caller to present, and an error
// if any occurred, instead of exiting the program directly.
//
// The upstream paths this package cannot reach are dropped, because Migrate is
// the only caller and never asks for them: schema export, the current-file
// diff, the before-apply hook, and the SQL Server statement suffix. Consult
// sqldef itself if one of them ever becomes necessary here.
func runMigration(generatorMode schema.GeneratorMode, db database.Database, sqlParser database.Parser, options *sqldef.Options) (migrated bool, advisory string, err error) {
	// Set the generator config on the database for privilege filtering
	// Note: MySQL will populate MysqlLowerCaseTableNames from the server
	db.SetGeneratorConfig(options.Config)
	options.Config = db.GetGeneratorConfig()

	currentDDLs, exportErr := db.ExportDDLs()
	if exportErr != nil {
		return false, "", errors.Wrap(exportErr, "failed to export ddls")
	}

	defaultSchema := db.GetDefaultSchema()

	ddls, genErr := schema.GenerateIdempotentDDLs(generatorMode, sqlParser, options.DesiredDDLs, currentDDLs, options.Config, defaultSchema)
	if genErr != nil {
		return false, "", genErr
	}
	if len(ddls) == 0 {
		return false, "", nil
	}

	// Detect verified table and index renames for the caller to present
	// alongside the plan. Detection guides only; nothing is rewritten or
	// executed here. Table renames come first: their DROP TABLE would discard
	// data, so they are the ones a reviewer must act on before anything else.
	if generatorMode == schema.GeneratorModeMysql {
		advisory = combineAdvisories(
			formatTableRenames(detectTableRenames(generatorMode, sqlParser, options.Config, defaultSchema, ddls, currentDDLs)),
			formatIndexRenames(detectIndexRenames(ddls, currentDDLs)),
		)
	}

	if options.DryRun {
		dryRunDB, dryRunErr := newDryRunDatabase(db)
		if dryRunErr != nil {
			return false, "", dryRunErr
		}
		defer dryRunDB.Close()
		db = dryRunDB
	}

	err = database.RunDDLs(db, ddls, "", "", database.StdoutLogger{})
	if err != nil {
		return false, "", err
	}
	return true, advisory, nil
}

var (
	dryRunDatabaseWrapperMu sync.Mutex
	dryRunDatabaseWrappers  []*dryRunDatabaseWrapper
)

type dryRunDatabaseWrapper struct {
	database.Database
}

func newDryRunDatabase(db database.Database) (*database.DryRunDatabase, error) {
	wrapper := &dryRunDatabaseWrapper{Database: db}

	// sqldef derives dry-run driver names from the wrapped DB pointer.
	// Keep wrappers alive so a later dry-run cannot reuse the same address.
	dryRunDatabaseWrapperMu.Lock()
	dryRunDatabaseWrappers = append(dryRunDatabaseWrappers, wrapper)
	dryRunDatabaseWrapperMu.Unlock()

	return database.NewDryRunDatabase(wrapper)
}
