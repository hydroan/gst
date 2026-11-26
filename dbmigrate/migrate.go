// Package dbmigrate renders registered Go models into a target schema and
// migrates a database towards it.
//
// SchemaDumper produces the target schema from the models themselves, so it
// always matches the DDL the runtime applies. Migrate then diffs that schema
// against the live database through sqldef and applies the difference, or
// only plans it in dry-run mode. The plan stays inside the tables the models
// declare: a table of the database no model declares is reported and left
// alone. What a plan would lose is left to a human: a plan dropping a column
// is applied only when the data loss is accepted, and a table or a column
// that looks renamed comes back as a Rename, carried out once confirmed.
package dbmigrate

import (
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/tlsconfig"
	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/database/mysql"
	"github.com/sqldef/sqldef/v3/database/postgres"
	"github.com/sqldef/sqldef/v3/parser"
	"github.com/sqldef/sqldef/v3/schema"
)

// DatabaseConfig is the connection the migration runs against. A MySQL
// target is reached under the TLS keys of the application's mysql section
// (config.App.MySQL), the way the application reaches it: encrypted when the
// section turns TLS on, plaintext otherwise.
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

// SQLiteTarget returns the migration target of a sqlite configuration: the
// database file the application opens, the one the path names. It reports
// false for a configuration selecting the in-memory database, with is_memory
// set, with no path, or with a path naming memory: the application creates
// that database's tables from the models every time it starts, and a
// migration, running in a process of its own, cannot reach it. For
// {Path: "./data.db"} it returns {Database: "./data.db"} and true; for
// {IsMemory: true, Path: "./data.db"} it returns nil and false.
func SQLiteTarget(cfg config.Sqlite) (*DatabaseConfig, bool) {
	if dbruntime.SQLiteInMemory(cfg) {
		return nil, false
	}
	return &DatabaseConfig{Database: cfg.Path}, true
}

// MigrateOptions tunes a single migration run.
type MigrateOptions struct {
	// DryRun plans the migration and reports what would change without
	// touching the database.
	DryRun bool
	// AcceptDataLoss lets the run apply a plan that drops columns, and with
	// them the data they hold. Without it such a plan is refused whole.
	AcceptDataLoss bool
	// IgnoreIndexes names the indexes the plan keeps although no model
	// declares them, each as table.index, in whatever case: an index made by
	// hand for what a model's Indexes cannot declare, a full-text index, one
	// over an expression. The plan leaves such an index as the database
	// holds it and lists it under Plan.KeptIndexes. The models come first:
	// an entry naming an index a model declares changes nothing of what the
	// plan makes of that index, and is listed under Plan.RedundantIgnores.
	// An entry naming an index the database lacks, or a table no model
	// declares, names nothing to keep.
	IgnoreIndexes []string
}

// Plan is what a migration would do to the database: the statements it would
// run, in order, and what a reviewer has to know beside them.
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
	// Renames are the renames the plan suspects where its statements drop
	// and add, tables first. The statements carry none of them out: see
	// Rename and ApplyRenames.
	Renames []Rename
	// DroppedColumns are the columns the statements drop, with the data they
	// hold, each as table.column.
	DroppedColumns []string
	// UndeclaredTables are the tables of the database no model declares. The
	// plan leaves them as they are.
	UndeclaredTables []string
	// KeptIndexes are the indexes no model declares that the plan leaves as
	// they are because MigrateOptions.IgnoreIndexes names them, each as
	// table.index.
	KeptIndexes []string
	// RedundantIgnores are the entries of MigrateOptions.IgnoreIndexes naming
	// an index a model declares, each as table.index: the model decides what
	// becomes of that index, and the entry keeps nothing.
	RedundantIgnores []string
	// CreatesDatabase reports that the plan starts from a sqlite database file
	// that does not exist yet: applying its statements creates the file.
	// MySQL and PostgreSQL databases must exist, so it is always false there.
	CreatesDatabase bool
}

// Changed reports whether the plan has anything to run: a database already
// matching the models plans nothing.
func (p Plan) Changed() bool { return len(p.Statements) > 0 }

// Migrate plans the schema changes towards the models' schema, and applies
// them unless the option asks for a dry run. It returns the plan either way,
// so a caller that plans first can execute exactly what it showed through
// Apply.
//
// The plan stays inside the tables the models declare. A table of the
// database no model declares is never dropped nor changed: the plan lists it
// under UndeclaredTables. Inside a table the models declare, a column and an
// index the models do not declare are dropped, but for the indexes the
// option names under IgnoreIndexes, which the plan leaves as they are and
// lists under KeptIndexes. Dropping an index loses no data; dropping a
// column does, so the plan lists the column under DroppedColumns, and a run
// that is no dry run refuses the whole plan unless the option accepts the
// data loss.
//
// A dry run creates nothing. A sqlite database file that does not exist yet
// is planned against as the empty database it would be, and the plan reports
// CreatesDatabase; the file comes into being when the plan is applied. A file
// whose directory does not exist either fails the migration: the driver
// creates the file but not the directory.
//
// A rename is never told from the two schemas alone: a table under a new
// name arrives as a table the models lack beside one the database lacks, a
// column as a column to drop beside one to add. Where the two sides of such
// a pair hold the same definition, the plan reports the pair under Renames,
// with the statement that renames; the statements of the plan still drop
// and add, until ApplyRenames has carried the rename out and the migration
// is planned again.
//
// An index is different, holding no data of its own: one the models declare
// under a name the database lacks, while the database holds an index of the
// same definition under a name no model declares, is planned as the rename
// of that index, on MySQL and PostgreSQL. SQLite renames no index, so the
// plan drops it and creates it there. Such a rename must run through this
// migration path BEFORE deploying code that carries the new index name: once
// it is applied, startup table preparation matches the new name and does
// nothing. With database.auto_migrate enabled (local development, tests),
// deploying first instead fails the start: table preparation finds the same
// definition under the old name and refuses with the rename statement,
// dropping nothing. With auto_migrate disabled (the production default) the
// start passes, but the model and the database keep drifting until the
// migration runs.
//
// A SQLite database whose columns no longer match the models' definitions —
// type, nullability, default or check — fails the migration instead of
// planning nothing: SQLite's ALTER TABLE cannot change a column, and sqldef
// plans no statement for it. The error names the table and the columns and
// points to the rebuild SQLite documents for such changes.
func Migrate(schemas []string, dbtyp config.DBType, cfg *DatabaseConfig, opt *MigrateOptions) (plan Plan, err error) {
	if len(schemas) == 0 || cfg == nil {
		return Plan{}, nil
	}
	if opt == nil {
		opt = &MigrateOptions{}
	}

	// Opening a sqlite database file creates it, so a dry run plans against a
	// missing one through an empty database of its own. A connection string
	// naming no file, an empty one or one naming the in-memory database, has
	// no file to miss.
	var missing bool
	if dbtyp == config.DBSqlite && !dbruntime.SQLiteInMemory(config.Sqlite{Path: cfg.Database}) {
		if missing, err = sqliteFileMissing(cfg.Database); err != nil {
			return Plan{}, err
		}
	}
	target := cfg
	if missing && opt.DryRun {
		target = &DatabaseConfig{Database: emptySQLiteDatabase}
	}

	db, parseMode, genMode, err := openTarget(dbtyp, target)
	if err != nil {
		return Plan{}, err
	}
	defer db.Close()

	if plan, err = planMigration(genMode, db, database.NewParser(parseMode), strings.Join(schemas, ";\n"), opt.IgnoreIndexes); err != nil {
		return Plan{}, err
	}
	plan.CreatesDatabase = missing && plan.Changed()
	if opt.DryRun || !plan.Changed() {
		return plan, nil
	}
	if err = refuseDataLoss(plan, opt.AcceptDataLoss); err != nil {
		return Plan{}, err
	}
	if err = runStatements(db, dbtyp, plan.Statements, nil); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// ApplyOptions tunes how a plan is applied.
type ApplyOptions struct {
	// AcceptDataLoss lets Apply run a plan that drops columns, and with them
	// the data they hold. Without it such a plan is refused whole.
	AcceptDataLoss bool
	// Progress, when set, is called as each statement of the plan starts,
	// with the position of the statement in the plan, counted from zero.
	Progress func(index int, statement string)
}

// Apply runs the statements of a plan as they stand, against the same kind of
// target Migrate planned them for. Nothing is planned again: what runs is
// what the plan carries. A plan with nothing to run does nothing, and a plan
// dropping columns is refused whole unless the option accepts the data loss.
//
// A statement that fails ends the run, and the error says which statement it
// was and what is left of the ones before it: MySQL commits every statement
// on its own, so they stay applied, while PostgreSQL and SQLite run the plan
// in one transaction and roll all of it back.
func Apply(plan Plan, dbtyp config.DBType, cfg *DatabaseConfig, opt *ApplyOptions) error {
	if !plan.Changed() || cfg == nil {
		return nil
	}
	if opt == nil {
		opt = &ApplyOptions{}
	}
	if err := refuseDataLoss(plan, opt.AcceptDataLoss); err != nil {
		return err
	}
	db, _, _, err := openTarget(dbtyp, cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	return runStatements(db, dbtyp, plan.Statements, opt.Progress)
}

// ApplyRenames carries out the renames a reviewer confirmed, in the order
// given, against the target Migrate planned for. The migration planned again
// afterwards finds each table and column under its new name, and plans what
// is left to change there.
func ApplyRenames(renames []Rename, dbtyp config.DBType, cfg *DatabaseConfig) error {
	statements := make([]string, 0, len(renames))
	for _, rename := range renames {
		statements = append(statements, rename.Statements...)
	}
	if len(statements) == 0 || cfg == nil {
		return nil
	}
	db, _, _, err := openTarget(dbtyp, cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	return runStatements(db, dbtyp, statements, nil)
}

// refuseDataLoss fails for a plan dropping columns unless the data loss was
// accepted, naming the columns.
func refuseDataLoss(plan Plan, accepted bool) error {
	if accepted || len(plan.DroppedColumns) == 0 {
		return nil
	}
	return errors.Newf("the migration drops %s and the data they hold: it is applied only when the data loss is accepted", strings.Join(plan.DroppedColumns, ", "))
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
	}

	var db database.Database
	var err error
	switch dbtyp {
	case config.DBMySQL:
		if config.App.MySQL.TLSEnabled {
			if err = registerMySQLTLS(config.App.MySQL); err != nil {
				return nil, 0, 0, err
			}
			dbcfg.SslMode = mysqlTLSName
		}
		db, err = mysql.NewDatabase(dbcfg)
		return db, parser.ParserModeMysql, schema.GeneratorModeMysql, err
	case config.DBPostgres:
		dbcfg.SslMode = cfg.SSLMode
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

// mysqlTLSName is the name the TLS configuration of a migration is
// registered with the MySQL driver under (see registerMySQLTLS).
const mysqlTLSName = "gst-migrate"

// registerMySQLTLS registers the TLS configuration cfg asks for with the
// MySQL driver under mysqlTLSName, the name sqldef then selects it by: sqldef
// takes the TLS setup of a connection by the name the driver knows it under,
// and builds none itself past a CA bundle. The configuration is the one the
// application's own connections run under (see tlsconfig.Build), so a
// migration reaches the server the way the application does.
func registerMySQLTLS(cfg config.MySQL) error {
	tlsConfig, err := tlsconfig.Build(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify)
	if err != nil {
		return errors.Wrap(err, "mysql tls")
	}
	if err = mysqldriver.RegisterTLSConfig(mysqlTLSName, tlsConfig); err != nil {
		return errors.Wrap(err, "failed to register the mysql tls configuration")
	}
	return nil
}

// planMigration plans the migration of db towards the models' schema: the
// statements sqldef generates inside the tables the models declare, and what
// the two schemas tell beside them, read by name off sqldef's own parser,
// the columns the statements drop, the tables no model declares and the
// renames the plan suspects. The indexes ignoreIndexes names are kept (see
// keepIndexes). Nothing is executed.
func planMigration(mode schema.GeneratorMode, db database.Database, sqlParser database.Parser, desiredDDLs string, ignoreIndexes []string) (Plan, error) {
	// The generator configuration goes through the database, which fills in
	// what the server decides: MySQL reports how it compares table names.
	// Drops are planned, the plan being the whole difference; what they lose
	// is reported beside it.
	db.SetGeneratorConfig(database.GeneratorConfig{EnableDrop: true})
	generatorConfig := db.GetGeneratorConfig()

	currentDDLs, err := db.ExportDDLs()
	if err != nil {
		return Plan{}, errors.Wrap(err, "failed to export ddls")
	}
	defaultSchema := db.GetDefaultSchema()

	desired, err := readShape(sqlParser, desiredDDLs)
	if err != nil {
		return Plan{}, errors.Wrap(err, "failed to parse the models' schema")
	}
	current, err := readShape(sqlParser, currentDDLs)
	if err != nil {
		return Plan{}, errors.Wrap(err, "failed to parse the current schema")
	}
	// A kept index is taken out of what the database is planned from, so
	// everything below finds it on neither side and plans nothing for it:
	// no drop, and no rename into an index of the same definition a model
	// declares under another name.
	kept, redundant, err := keepIndexes(current, desired, ignoreIndexes)
	if err != nil {
		return Plan{}, err
	}
	if len(kept) > 0 {
		if currentDDLs, err = withoutIndexes(sqlParser, currentDDLs, kept); err != nil {
			return Plan{}, err
		}
		if current, err = readShape(sqlParser, currentDDLs); err != nil {
			return Plan{}, errors.Wrap(err, "failed to parse the current schema without the indexes kept")
		}
	}

	// sqldef confines the plan to the tables named here, and matches a table
	// by the whole name it knows it under, which carries the schema on
	// PostgreSQL: public.records.
	generatorConfig.TargetTables = make([]string, 0, len(desired.names))
	for _, name := range desired.names {
		if defaultSchema != "" {
			name = defaultSchema + "." + name
		}
		generatorConfig.TargetTables = append(generatorConfig.TargetTables, regexp.QuoteMeta(name))
	}

	// SQLite renames no index, so an index under a new name is dropped and
	// created there, which is what sqldef plans unprompted.
	if mode != schema.GeneratorModeSQLite3 {
		desiredDDLs = annotateRenamedIndexes(desiredDDLs, renamedIndexes(current, desired))
	}
	statements, err := schema.GenerateIdempotentDDLs(mode, sqlParser, desiredDDLs, currentDDLs, generatorConfig, defaultSchema)
	if err != nil {
		return Plan{}, err
	}
	// sqldef plans no statement for a changed column definition on SQLite,
	// whose ALTER TABLE cannot express one, and would answer "no changes"
	// for a database that no longer matches the models. The migration
	// refuses instead, before an empty plan can pass for an up-to-date
	// database.
	if mode == schema.GeneratorModeSQLite3 {
		if err = refuseSQLiteColumnDrift(desired, current); err != nil {
			return Plan{}, err
		}
	}
	// A table the models declare and the database lacks is always planned.
	// An empty plan beside one means the names above matched no table, and
	// must not pass for an up-to-date database.
	if missing := tablesOnlyIn(desired, current); len(statements) == 0 && len(missing) > 0 {
		return Plan{}, errors.Newf("the migration planned nothing while the database lacks the table %q of the models", missing[0].name)
	}

	// The statements are held to what the plan says of the kept indexes: one
	// the generator still plans a change for is an index it reads otherwise
	// off the statement kept than off the database, and a plan dropping or
	// rebuilding what it lists as kept must not pass.
	for _, name := range kept {
		_, index, _ := strings.Cut(name, ".")
		mentions := regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_$])` + regexp.QuoteMeta(index) + `($|[^A-Za-z0-9_$])`)
		for _, statement := range statements {
			if mentions.MatchString(statement) {
				return Plan{}, errors.Newf("the migration cannot keep the index %s: the plan would still run %q; drop the entry and declare the index in its model, or migrate it by hand", name, statement)
			}
		}
	}

	plan := Plan{Statements: statements, KeptIndexes: kept, RedundantIgnores: redundant}
	for _, table := range tablesOnlyIn(current, desired) {
		plan.UndeclaredTables = append(plan.UndeclaredTables, table.name)
	}
	for _, name := range desired.names {
		have := current.table(name)
		if have == nil {
			continue
		}
		for _, column := range columnsOnlyIn(have, desired.table(name)) {
			plan.DroppedColumns = append(plan.DroppedColumns, have.name+"."+column.name)
		}
	}
	plan.Renames = comparison{mode: mode, parser: sqlParser, defaultSchema: defaultSchema}.renames(current, desired)
	return plan, nil
}

// runStatements executes statements against db in order through sqldef's
// runner, which keeps them on one connection and, on a database whose DDL
// runs in transactions, in one transaction. progress, when set, hears of
// each statement as it starts.
//
// A failure is reported with the statement it stopped at and what is left of
// the ones before it. sqldef's runner cannot say either: it returns the
// driver's error alone.
func runStatements(db database.Database, dbtyp config.DBType, statements []string, progress func(index int, statement string)) error {
	started := &startedStatements{statements: statements, progress: progress}
	err := database.RunDDLs(db, statements, "", "", started)
	if err == nil || started.count == 0 {
		return err
	}
	// MySQL commits every DDL statement on its own, in or out of a
	// transaction; PostgreSQL and SQLite roll the transaction back.
	if dbtyp == config.DBMySQL && started.count > 1 {
		return errors.Wrapf(err, "statement %d of %d failed, and the statements before it stay applied", started.count, len(statements))
	}
	return errors.Wrapf(err, "statement %d of %d failed, and nothing of the plan is applied", started.count, len(statements))
}

// startedStatements is the logger runStatements hands sqldef's runner, which
// prints every statement it is about to execute, as an argument of its own,
// among the transaction marks around them. It tells the statements of the
// run by comparing each printed argument with the statement due next, counts
// the ones started and reports each to progress.
type startedStatements struct {
	statements []string
	progress   func(index int, statement string)
	count      int
}

func (s *startedStatements) Print(...any) {}

func (s *startedStatements) Println(...any) {}

func (s *startedStatements) Printf(_ string, v ...any) {
	if s.count == len(s.statements) || len(v) != 1 {
		return
	}
	if statement, ok := v[0].(string); !ok || statement != s.statements[s.count] {
		return
	}
	if s.progress != nil {
		s.progress(s.count, s.statements[s.count])
	}
	s.count++
}
