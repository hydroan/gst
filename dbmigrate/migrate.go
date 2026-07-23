package dbmigrate

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hydroan/gst/config"
	"github.com/sqldef/sqldef/v3"
	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/database/mysql"
	"github.com/sqldef/sqldef/v3/database/postgres"

	// "github.com/sqldef/sqldef/v3/database/sqlite3"
	"github.com/sqldef/sqldef/v3/parser"
	"github.com/sqldef/sqldef/v3/schema"
)

type DatabaseConfig struct {
	Database string
	Username string
	Password string
	Host     string
	Port     int
	SSLMode  string
}

type MigrateOption struct {
	Schemas    []string
	DryRun     bool
	EnableDrop bool
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

// Migrate applies the schema changes to the database.
// It returns true if any changes were applied (or would be applied in dry-run mode),
// and false if the database schema is already up-to-date.
//
// Index renames must run through this migration path BEFORE deploying code
// that carries the new index name: once the rename is applied, bootstrap
// AutoMigrate matches the new name and does nothing. Deploying first instead
// makes gorm's MySQL driver silently DROP and re-CREATE single-column unique
// indexes during startup, which rebuilds the index with a full table scan and
// skips every review step.
//
// When a MySQL plan both drops and adds indexes on the same table, suspected
// renames are returned as advisory text with ready-to-run RENAME INDEX
// guidance. The caller owns when and how to present it; executing the
// rename stays a human decision.
func Migrate(schemas []string, dbtyp config.DBType, cfg *DatabaseConfig, opt *MigrateOption) (migrated bool, advisory string, err error) {
	if len(schemas) == 0 {
		return false, "", nil
	}
	if cfg == nil {
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
	}
	if err != nil {
		return false, "", err
	}
	defer db.Close()

	sqlParser := database.NewParser(parseMode)
	return run(genMode, db, sqlParser, migOpt)
}

// run executes the database migration logic.
// This function is derived from sqldef.Run (https://github.com/sqldef/sqldef),
// but modified to return a boolean indicating whether any migration was
// performed, the index-rename advisory text for the caller to present, and
// an error if any occurred, instead of exiting the program directly.
func run(generatorMode schema.GeneratorMode, db database.Database, sqlParser database.Parser, options *sqldef.Options) (migrated bool, advisory string, err error) {
	// Set the generator config on the database for privilege filtering
	// Note: MySQL will populate MysqlLowerCaseTableNames from the server
	db.SetGeneratorConfig(options.Config)
	options.Config = db.GetGeneratorConfig()

	currentDDLs, exportErr := db.ExportDDLs()
	if exportErr != nil {
		return false, "", fmt.Errorf("Error on ExportDDLs: %w", exportErr)
	}

	defaultSchema := db.GetDefaultSchema()

	var ddlSuffix string
	if generatorMode == schema.GeneratorModeMssql {
		ddlSuffix = "GO\n"
	} else {
		ddlSuffix = ""
	}

	if options.Export {
		if currentDDLs == "" {
			// fmt.Printf("-- No table exists --\n")
		} else {
			ddls, parseErr := schema.ParseDDLs(generatorMode, sqlParser, currentDDLs, defaultSchema)
			if parseErr != nil {
				return false, "", parseErr
			}
			ddls = schema.FilterTables(ddls, options.Config)
			ddls = schema.FilterViews(ddls, options.Config)
			ddls = schema.FilterPrivileges(ddls, options.Config)
			for i, ddl := range ddls {
				if i > 0 {
					fmt.Println()
				}
				fmt.Printf("%s;\n", ddl.Statement())
				fmt.Print(ddlSuffix)
			}
		}
		return false, "", nil
	}

	ddls, genErr := schema.GenerateIdempotentDDLs(generatorMode, sqlParser, options.DesiredDDLs, currentDDLs, options.Config, defaultSchema)
	if genErr != nil {
		return false, "", genErr
	}
	if len(ddls) == 0 {
		// fmt.Println("-- Nothing is modified --")
		return false, "", nil
	}

	// Detect verified index renames for the caller to present alongside the
	// plan. Detection guides only; nothing is rewritten or executed here.
	if generatorMode == schema.GeneratorModeMysql {
		advisory = formatIndexRenames(detectIndexRenames(ddls, currentDDLs))
	}

	if options.DryRun || len(options.CurrentFile) > 0 {
		dryRunDB, dryRunErr := newDryRunDatabase(db)
		if dryRunErr != nil {
			return false, "", dryRunErr
		}
		defer dryRunDB.Close()
		db = dryRunDB
	}

	err = database.RunDDLs(db, ddls, options.BeforeApply, ddlSuffix, database.StdoutLogger{})
	if err != nil {
		return false, "", err
	}
	return true, advisory, nil
}
