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

// Plan is what a migration would do to the database: the statements it would
// run, in order, and the rename advisory that belongs beside them.
//
// It exists so that what a reviewer approved is what runs. A plan is computed
// against the schema the database has at that moment; computing it a second
// time to execute it would plan against whatever the database has by then,
// which is not what was shown — a table created in between turns a CREATE
// into an ALTER, and one dropped in between turns nothing into a CREATE.
// Apply runs the statements of the plan as they stand.
type Plan struct {
	// Statements are the DDL statements of the plan, in execution order.
	Statements []string
	// Advisory is the rename advisory, empty when the plan suggests none.
	Advisory string
}

// Changed reports whether the plan has anything to run: a database already
// matching the models plans nothing.
func (p Plan) Changed() bool { return len(p.Statements) > 0 }

// Migrate plans the schema changes towards the models' schema, and applies
// them unless the option asks for a dry run. It returns the plan either way,
// so a caller that plans first can execute exactly what it showed through
// Apply.
//
// Index renames must run through this migration path BEFORE deploying code
// that carries the new index name: once the rename is applied, startup table
// preparation matches the new name and does nothing. With database.auto_migrate
// enabled (local development, tests), deploying first instead fails the start:
// table preparation finds the same definition under the old name and refuses
// with the rename statement, dropping nothing. With auto_migrate disabled (the
// production default) the start passes, but the model and the database keep
// drifting until the migration runs.
//
// When a MySQL or PostgreSQL plan drops and re-creates an identical
// definition — an index on the same table, or a whole table under a new name
// — the suspected renames are returned as advisory text with ready-to-run
// rename statements in that server's syntax. For tables the advisory doubles
// as a data-loss guard, because the planned DROP TABLE would discard every
// row that the metadata-only rename keeps. The caller owns when and how to
// present it; executing the rename stays a human decision.
func Migrate(schemas []string, dbtyp config.DBType, cfg *DatabaseConfig, opt *MigrateOption) (plan Plan, err error) {
	if len(schemas) == 0 || cfg == nil {
		return Plan{}, nil
	}
	if opt == nil {
		opt = &MigrateOption{}
	}

	migOpt := &sqldef.Options{
		DryRun:      opt.DryRun,
		DesiredDDLs: strings.Join(schemas, ";\n"),
		Config: database.GeneratorConfig{
			EnableDrop: opt.EnableDrop,
		},
	}

	db, parseMode, genMode, err := openTarget(dbtyp, cfg)
	if err != nil {
		return Plan{}, err
	}
	defer db.Close()

	sqlParser := database.NewParser(parseMode)
	return runMigration(genMode, db, sqlParser, migOpt)
}

// Apply runs the statements of a plan as they stand, against the same kind of
// target Migrate planned them for. Nothing is planned again: what runs is
// what the plan carries, including the destructive statements a reviewer
// approved. A plan with nothing to run does nothing.
func Apply(plan Plan, dbtyp config.DBType, cfg *DatabaseConfig) error {
	if !plan.Changed() || cfg == nil {
		return nil
	}
	db, _, _, err := openTarget(dbtyp, cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	return database.RunDDLs(db, plan.Statements, "", "", database.StdoutLogger{})
}

// openTarget opens the database a migration runs against, with the parser and
// generator modes of its dialect.
func openTarget(dbtyp config.DBType, cfg *DatabaseConfig) (database.Database, parser.ParserMode, schema.GeneratorMode, error) {
	dbcfg := database.Config{
		DbName:   cfg.Database,
		User:     cfg.Username,
		Password: cfg.Password,
		Host:     cfg.Host,
		Port:     cfg.Port,
		SslMode:  cfg.SSLMode,
	}

	var db database.Database
	var err error
	switch dbtyp {
	case config.DBMySQL:
		db, err = mysql.NewDatabase(dbcfg)
		return db, parser.ParserModeMysql, schema.GeneratorModeMysql, err
	case config.DBPostgres:
		db, err = postgres.NewDatabase(dbcfg)
		return db, parser.ParserModePostgres, schema.GeneratorModePostgres, err
	case config.DBSqlite:
		db, err = newSQLiteDatabase(dbcfg)
		return db, parser.ParserModeSQLite3, schema.GeneratorModeSQLite3, err
	default:
		// ClickHouse (and any other analytical store) is deliberately not
		// migratable here: its schema is a query-model design — engine,
		// ORDER BY, partitioning, TTL — that cannot be derived from Go models,
		// so the application owns it through hand-written DDL.
		return nil, 0, 0, errors.Newf("schema migration does not support %q: its schema is managed by hand-written DDL on the application side", dbtyp)
	}
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
func runMigration(generatorMode schema.GeneratorMode, db database.Database, sqlParser database.Parser, options *sqldef.Options) (plan Plan, err error) {
	// Set the generator config on the database for privilege filtering
	// Note: MySQL will populate MysqlLowerCaseTableNames from the server
	db.SetGeneratorConfig(options.Config)
	options.Config = db.GetGeneratorConfig()

	currentDDLs, exportErr := db.ExportDDLs()
	if exportErr != nil {
		return Plan{}, errors.Wrap(exportErr, "failed to export ddls")
	}

	defaultSchema := db.GetDefaultSchema()

	ddls, genErr := schema.GenerateIdempotentDDLs(generatorMode, sqlParser, options.DesiredDDLs, currentDDLs, options.Config, defaultSchema)
	if genErr != nil {
		return Plan{}, genErr
	}
	if len(ddls) == 0 {
		return Plan{}, nil
	}
	plan = Plan{Statements: ddls}

	// Detect verified table and index renames for the caller to present
	// alongside the plan. Detection guides only; nothing is rewritten or
	// executed here. Table renames come first: their DROP TABLE would discard
	// data, so they are the ones a reviewer must act on before anything else.
	// SQLite stays out: it backs local development and tests, where databases
	// are created fresh and hold no schema worth renaming.
	if generatorMode == schema.GeneratorModeMysql || generatorMode == schema.GeneratorModePostgres {
		plan.Advisory = combineAdvisories(
			formatTableRenames(generatorMode, detectTableRenames(generatorMode, sqlParser, options.Config, defaultSchema, ddls, currentDDLs)),
			formatIndexRenames(generatorMode, detectIndexRenames(ddls, currentDDLs)),
		)
	}

	if options.DryRun {
		dryRunDB, dryRunErr := newDryRunDatabase(db)
		if dryRunErr != nil {
			return Plan{}, dryRunErr
		}
		defer dryRunDB.Close()
		db = dryRunDB
	}

	if err = database.RunDDLs(db, ddls, "", "", database.StdoutLogger{}); err != nil {
		return Plan{}, err
	}
	return plan, nil
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
