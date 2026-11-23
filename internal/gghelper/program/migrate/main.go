// The migration program migrates a project's database to its models, or
// dumps the schema its models describe: it is what gg migrate builds and runs
// inside the project. gg embeds it (see gghelper.MigrateProgram), writes it
// next to a generated project.go that imports the project's packages the
// way the project's main.go does, so every model the running service
// registers is in, and builds it with the project's dependency versions. It
// is written as project code: it imports the framework's public packages
// only. The mode and the schema source come in as flags, so the source stays
// the same between runs and the program Go built is reused.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dbmigrate"
	"github.com/hydroan/gst/model"

	// bootstrap links every framework package the running service links, so
	// this program sees the same registrations, and brings up the router and
	// the modules the way the service does.
	"github.com/hydroan/gst/bootstrap"
)

// The flags gg passes: the project's module path, the mode, for a schema
// dump of part of the models its source and the Go files under it, as gg
// listed them, and whether the terminal takes colors.
var (
	migrateModule            string
	migrateDryRun            bool
	migrateYes               bool
	migrateAcceptDataLoss    bool
	migrateSchemaOnly        bool
	migrateSchemaSource      string
	migrateSchemaSourceFiles fileList
	migrateColor             bool
)

// fileList collects the values of a flag given more than once.
type fileList []string

func (l *fileList) String() string { return strings.Join(*l, ", ") }

func (l *fileList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func main() {
	flag.StringVar(&migrateModule, "module", "", "the project's module path")
	flag.BoolVar(&migrateDryRun, "dry-run", false, "preview the migration SQL without applying it")
	flag.BoolVar(&migrateYes, "yes", false, "apply the migration without prompting for confirmation")
	flag.BoolVar(&migrateAcceptDataLoss, "accept-data-loss", false, "accept that the migration drops columns and the data they hold; required for such a migration under -yes")
	flag.BoolVar(&migrateSchemaOnly, "schema-only", false, "dump the schema and stop")
	flag.StringVar(&migrateSchemaSource, "schema-source", "", "the model source the schema dump covers; empty covers every registered model")
	flag.Var(&migrateSchemaSourceFiles, "schema-source-file", "a Go file under schema-source; repeat for each")
	flag.BoolVar(&migrateColor, "color", false, "color the output, for a terminal that takes colors")
	flag.Parse()

	// Load the configuration and initialize the router and modules, with
	// stdout suppressed during initialization to avoid cluttering the
	// migration output.
	initConfigRouterAndModules()

	// Collect all registered models.
	models := deduplicateModels(model.RegisteredModels())

	if migrateSchemaOnly {
		if err := runSchemaDump(models); err != nil {
			exitWithError(err)
		}
		return
	}

	// Dump the schema for the collected models.
	schema, err := dumpSchema(models)
	if err != nil {
		exitWithError(err)
	}

	// Get the database the migration targets, if the configuration has one
	// to migrate.
	dbConfig, migratable := getDatabaseConfig()

	// Write the schema to a generated file for reference or debugging.
	schemaFile, err := writeSchemaFile(schema, len(models))
	if err != nil {
		exitWithError(err)
	}

	printMigrationSummary(schemaFile, dbConfig, len(models))

	if !migratable {
		done("Nothing to migrate: the application creates the tables of its in-memory database from the models every time it starts")
		note("To migrate a database file, set sqlite.is_memory = false and point sqlite.path at the file")
		return
	}

	// Perform migration.
	if err := performMigration(schema, dbConfig); err != nil {
		exitWithError(err)
	}
}

// exitWithError prints the error as the line a failed command ends on and
// exits without a panic stack.
func exitWithError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "\n%s %v\n", paint(os.Stderr, colorRed, symbolError), err)
	os.Exit(1)
}

// initConfigRouterAndModules loads the configuration and initializes the router
// and modules (see bootstrap.InitRouterAndModules), so every model the project
// and its modules register is in. It temporarily suppresses stdout to prevent
// initialization logs from appearing in the console.
func initConfigRouterAndModules() {
	oldStdout := os.Stdout
	null, err := os.Open(os.DevNull)
	if err != nil {
		exitWithError(err)
	}
	os.Stdout = null
	defer func() {
		os.Stdout = oldStdout
		null.Close()
	}()

	if err = config.Init(); err != nil {
		exitWithError(err)
	}
	if err = bootstrap.InitRouterAndModules(); err != nil {
		exitWithError(err)
	}
}

// deduplicateModels keeps the first registered model for each concrete model type.
func deduplicateModels(models []any) []any {
	unique := make([]any, 0, len(models))
	seen := make(map[reflect.Type]struct{}, len(models))
	for _, item := range models {
		typ := reflect.TypeOf(item)
		if typ == nil {
			unique = append(unique, item)
			continue
		}
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if _, exists := seen[typ]; exists {
			continue
		}
		seen[typ] = struct{}{}
		unique = append(unique, item)
	}
	return unique
}

type modelTypeKey struct {
	PkgPath string
	Name    string
}

func runSchemaDump(models []any) error {
	var err error
	models, err = filterModelsBySource(models, migrateSchemaSource, []string(migrateSchemaSourceFiles))
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return errors.New("no registered models found")
	}

	schema, err := dumpSchema(models)
	if err != nil {
		return err
	}
	printSchemaDump(schema, len(models), migrateSchemaSource)
	return nil
}

func filterModelsBySource(models []any, source string, files []string) ([]any, error) {
	if source == "" {
		return models, nil
	}

	targetTypes, err := collectSourceModelTypes(source, files)
	if err != nil {
		return nil, err
	}

	selected := make([]any, 0, len(models))
	for _, item := range models {
		key, ok := modelKey(item)
		if !ok {
			continue
		}
		if _, exists := targetTypes[key]; exists {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		return nil, errors.Newf("no registered models found under %s", source)
	}
	return selected, nil
}

func modelKey(model any) (modelTypeKey, bool) {
	typ := reflect.TypeOf(model)
	if typ == nil {
		return modelTypeKey{}, false
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.PkgPath() == "" || typ.Name() == "" {
		return modelTypeKey{}, false
	}
	return modelTypeKey{
		PkgPath: typ.PkgPath(),
		Name:    typ.Name(),
	}, true
}

func collectSourceModelTypes(source string, files []string) (map[modelTypeKey]struct{}, error) {
	targetTypes := make(map[modelTypeKey]struct{})
	for _, file := range files {
		if err := collectFileModelTypes(file, targetTypes); err != nil {
			return nil, err
		}
	}
	if len(targetTypes) == 0 {
		return nil, errors.Newf("no model type declarations found under %s", source)
	}
	return targetTypes, nil
}

func collectFileModelTypes(filename string, targetTypes map[modelTypeKey]struct{}) error {
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		return errors.Wrapf(err, "failed to parse model source %s", filename)
	}
	pkgPath, err := modelPackagePath(filename)
	if err != nil {
		return err
	}

	for _, decl := range parsed.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			targetTypes[modelTypeKey{PkgPath: pkgPath, Name: typeSpec.Name.Name}] = struct{}{}
		}
	}
	return nil
}

func modelPackagePath(filename string) (string, error) {
	absFile, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, absFile)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.Newf("model source %s is outside project root", filename)
	}

	dir := filepath.Dir(rel)
	if dir == "." {
		return migrateModule, nil
	}
	return migrateModule + "/" + filepath.ToSlash(dir), nil
}

func printSchemaDump(schema string, modelCount int, source string) {
	section("Model Schema")
	item("Database: %s", config.App.Database.Type)
	if strings.TrimSpace(source) != "" {
		item("Source: %s", source)
	}
	item("Models: %d", modelCount)
	fmt.Println()
	fmt.Println(strings.TrimRight(schema, "\n"))
}

// dumpSchema creates a schema dump for the provided models using the configured database type.
func dumpSchema(models []any) (string, error) {
	dumper, err := dbmigrate.NewSchemaDumper()
	if err != nil {
		return "", err
	}
	dbtyp := config.App.Database.Type
	return dumper.Dump(dbtyp, models...)
}

// writeSchemaFile writes the generated schema snapshot under the database-specific migration directory.
func writeSchemaFile(schema string, modelCount int) (string, error) {
	schemaDir := filepath.Join("generated", "migrate", string(config.App.Database.Type))
	schemaFile := filepath.Join(schemaDir, "schema.sql")
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		return "", errors.Wrapf(err, "failed to create %s directory", schemaDir)
	}
	// #nosec G306 -- the schema snapshot is a project file the developer reads and commits.
	if err := os.WriteFile(schemaFile, []byte(schemaSnapshotContent(schema, modelCount)), 0o644); err != nil {
		return "", errors.Wrapf(err, "failed to write %s", schemaFile)
	}
	return schemaFile, nil
}

// schemaSnapshotContent prefixes the dumped DDL with stable metadata for review.
func schemaSnapshotContent(schema string, modelCount int) string {
	header := fmt.Sprintf(
		"-- Code generated by gg migrate; DO NOT EDIT.\n"+
			"--\n"+
			"-- Module: %s\n"+
			"-- Database: %s\n"+
			"-- Registered models: %d\n"+
			"--\n"+
			"-- This file is a target schema snapshot generated from registered models.\n"+
			"-- It is not a migration plan and does not describe operations to apply.\n\n",
		migrateModule,
		config.App.Database.Type,
		modelCount,
	)
	return header + strings.TrimLeft(schema, "\n")
}

// getDatabaseConfig constructs the database configuration based on the
// application config. It reports false for an in-memory sqlite database,
// which has nothing to migrate: see dbmigrate.SQLiteTarget.
func getDatabaseConfig() (cfg *dbmigrate.DatabaseConfig, migratable bool) {
	switch config.App.Database.Type {
	case config.DBMySQL:
		cfg = &dbmigrate.DatabaseConfig{
			Host:     config.App.MySQL.Host,
			Port:     int(config.App.MySQL.Port),
			Database: config.App.MySQL.Database,
			Username: config.App.MySQL.Username,
			Password: config.App.MySQL.Password,
		}
	case config.DBPostgres:
		cfg = &dbmigrate.DatabaseConfig{
			Host:     config.App.Postgres.Host,
			Port:     int(config.App.Postgres.Port),
			Database: config.App.Postgres.Database,
			Username: config.App.Postgres.Username,
			Password: config.App.Postgres.Password,
			SSLMode:  config.App.Postgres.SSLMode,
		}
	case config.DBSqlite:
		return dbmigrate.SQLiteTarget(config.App.Sqlite)
	default:
		exitWithError(errors.Newf("unsupported database type: %s", config.App.Database.Type))
	}
	return cfg, true
}

// printMigrationSummary prints the target and generated schema path before any SQL is applied.
func printMigrationSummary(schemaFile string, cfg *dbmigrate.DatabaseConfig, modelCount int) {
	section("Migrate")
	item("Database: %s", config.App.Database.Type)
	item("Target: %s", databaseTarget(cfg))
	item("Models: %d", modelCount)
	item("Schema: %s", schemaFile)
}

// databaseTarget formats the configured database target without printing
// credentials: the file on sqlite, "in-memory database" when there is none.
func databaseTarget(cfg *dbmigrate.DatabaseConfig) string {
	switch config.App.Database.Type {
	case config.DBSqlite:
		if cfg == nil {
			return "in-memory database"
		}
		return cfg.Database
	default:
		return fmt.Sprintf("%s:%d/%s", cfg.Host, cfg.Port, cfg.Database)
	}
}

// performMigration plans the migration, offers the renames the plan
// suspects, shows the plan with what a reviewer has to know beside it, and
// applies it once confirmed. It ends on what came of it: nothing to migrate,
// a dry run, or the statements applied and the time they took.
func performMigration(schema string, cfg *dbmigrate.DatabaseConfig) error {
	dbtyp := config.App.Database.Type

	// Planned against the schema the database has at that moment. What is
	// applied below is the plan shown, statement for statement: planning
	// again to execute would plan against whatever the database has by then,
	// which is not what was approved.
	planned := func() (dbmigrate.Plan, error) {
		return dbmigrate.Migrate([]string{schema}, dbtyp, cfg, &dbmigrate.MigrateOption{DryRun: true})
	}
	plan, err := planned()
	if err != nil {
		return err
	}

	// A rename is asked about before the plan is shown: carried out, it
	// leaves the plan without the drop and the add that stood for it, so the
	// migration is planned again from what the database then holds, where a
	// renamed table may show a renamed column of its own. A rename declined
	// is not asked about twice, and a run that cannot ask carries none out.
	declined := make(map[string]bool)
	for !migrateDryRun && !migrateYes {
		confirmed := confirmRenames(plan.Renames, declined)
		if len(confirmed) == 0 {
			break
		}
		if err = dbmigrate.ApplyRenames(confirmed, dbtyp, cfg); err != nil {
			return err
		}
		success("Renamed")
		if plan, err = planned(); err != nil {
			return err
		}
	}

	if !plan.Changed() {
		if len(plan.UndeclaredTables) > 0 {
			section("Plan")
			item("Left as they are, the tables no model declares: %s", strings.Join(plan.UndeclaredTables, ", "))
		}
		done("Nothing to migrate: the database matches the models")
		return nil
	}

	// Each statement goes under its position and ends in a semicolon, the
	// positions as comments, so the block pastes into a database client as
	// it stands; the line after them counts the statements by kind.
	section("Plan")
	for i, statement := range plan.Statements {
		fmt.Printf("  %s\n  %s;\n", paint(os.Stdout, colorGrey, fmt.Sprintf("-- [%d/%d]", i+1, len(plan.Statements))), strings.ReplaceAll(statement, "\n", "\n  "))
	}
	fmt.Printf("  %s\n", planSummary(plan.Statements))
	if plan.CreatesDatabase {
		item("The target database file does not exist yet: the plan starts from an empty database, and applying it creates the file.")
	}
	if len(plan.UndeclaredTables) > 0 {
		item("Left as they are, the tables no model declares: %s", strings.Join(plan.UndeclaredTables, ", "))
	}

	// The renames no one was asked about stay visible beside the plan that
	// drops and adds in their place.
	if pending := undecided(plan.Renames, declined); len(pending) > 0 {
		section("Suspected Renames")
		for _, rename := range pending {
			printRename(rename)
		}
		item("The plan above drops and adds instead. Run gg migrate without --dry-run and --yes to be asked about each rename, or run its statement yourself, then migrate again.")
	}

	// Dropping a column loses the data it holds, which someone has to accept:
	// the one answering the confirmation below, which then names the loss, or,
	// where no one is asked, --accept-data-loss.
	dropsData := len(plan.DroppedColumns) > 0
	if dropsData {
		section("Data Loss")
		item("The plan drops these columns and the data they hold: %s", strings.Join(plan.DroppedColumns, ", "))
		switch {
		case migrateAcceptDataLoss:
			item("--accept-data-loss is given: they are dropped when the plan is applied.")
		case migrateDryRun || migrateYes:
			item("Declare a column in its model to keep it. To drop them, confirm when gg migrate asks, or pass --accept-data-loss with --yes.")
		default:
			item("Declare a column in its model to keep it.")
		}
	}

	if migrateDryRun {
		done("Dry run: %s planned, nothing applied", counted(len(plan.Statements), "statement"))
		return nil
	}
	accepted := migrateAcceptDataLoss
	if migrateYes {
		if dropsData && !accepted {
			return errors.New("the migration was not applied: it drops columns, and --accept-data-loss is not given")
		}
	} else {
		question := "Apply the migration to the target database?"
		if dropsData && !accepted {
			question = "Apply the migration and drop the columns above with the data they hold?"
		}
		if !confirm("\n", question) {
			item("Migration canceled: nothing was applied")
			return nil
		}
		// The question named the loss, so the yes accepts it.
		accepted = true
	}

	// The statements shown above, as they stand. A terminal shows which one
	// is running, on one line the next one takes over; the line is gone once
	// the last is done, and nothing of it reaches a pipe or a file.
	onTerminal := isTerminal(os.Stdout)
	running := -1
	started := time.Now()
	err = dbmigrate.Apply(plan, dbtyp, cfg, &dbmigrate.ApplyOption{
		AcceptDataLoss: accepted,
		Progress: func(index int, _ string) {
			running = index
			if onTerminal {
				fmt.Printf("\r\x1b[2K  %s Applying %d/%d", paint(os.Stdout, colorGrey, symbolItem), index+1, len(plan.Statements))
			}
		},
	})
	if onTerminal && running >= 0 {
		fmt.Print("\r\x1b[2K")
	}
	if err != nil {
		if running < 0 {
			return err
		}
		// The error says which statement failed and what is left of the
		// ones before it; the statement itself and the way on follow it.
		return errors.Newf("%v\n    %s;\n  Run gg migrate again once the cause is gone: it plans from what the database then holds.",
			err, strings.ReplaceAll(plan.Statements[running], "\n", "\n    "))
	}
	done("Applied %s (%s)", counted(len(plan.Statements), "statement"), elapsed(time.Since(started)))
	return nil
}

// statementKinds are the openings planSummary counts the statements of a
// plan by, in the order it names them; one statement opens with one of them
// at most, the longer of two that share a start standing first.
var statementKinds = []string{
	"CREATE TABLE", "CREATE UNIQUE INDEX", "CREATE INDEX",
	"ALTER TABLE", "ALTER INDEX",
	"DROP TABLE", "DROP INDEX",
}

// planSummary counts the statements of a plan by the words they open with,
// the kinds none opens with left out and whatever opens otherwise counted
// last:
//
//	22 statements: 13 CREATE TABLE, 4 CREATE UNIQUE INDEX, 5 CREATE INDEX
//	3 statements: 2 ALTER TABLE, 1 other
func planSummary(statements []string) string {
	counts := make([]int, len(statementKinds)+1)
	for _, statement := range statements {
		opening := strings.ToUpper(strings.Join(strings.Fields(statement), " "))
		kind := len(statementKinds)
		for i, name := range statementKinds {
			if strings.HasPrefix(opening, name+" ") {
				kind = i
				break
			}
		}
		counts[kind]++
	}
	var kinds []string
	for i, count := range counts {
		switch {
		case count == 0:
		case i == len(statementKinds):
			kinds = append(kinds, fmt.Sprintf("%d other", count))
		default:
			kinds = append(kinds, fmt.Sprintf("%d %s", count, statementKinds[i]))
		}
	}
	return counted(len(statements), "statement") + ": " + strings.Join(kinds, ", ")
}

// confirmRenames asks about each rename the plan suspects that was not
// declined before, and returns the ones confirmed; a rename declined is
// recorded in declined.
func confirmRenames(renames []dbmigrate.Rename, declined map[string]bool) []dbmigrate.Rename {
	pending := undecided(renames, declined)
	if len(pending) == 0 {
		return nil
	}
	section("Suspected Renames")
	confirmed := make([]dbmigrate.Rename, 0, len(pending))
	for _, rename := range pending {
		printRename(rename)
		if confirm("", fmt.Sprintf("Rename %s now, by the statement above?", rename)) {
			confirmed = append(confirmed, rename)
		} else {
			declined[rename.String()] = true
		}
	}
	return confirmed
}

// undecided returns the renames no answer declined.
func undecided(renames []dbmigrate.Rename, declined map[string]bool) []dbmigrate.Rename {
	pending := make([]dbmigrate.Rename, 0, len(renames))
	for _, rename := range renames {
		if !declined[rename.String()] {
			pending = append(pending, rename)
		}
	}
	return pending
}

// printRename prints a rename the plan suspects, what the plan does in its
// place, and the statement that renames:
//
//	→ column samples.title to subject: both hold the same definition. Left as it is, the plan drops title with its data and adds subject empty.
//	    ALTER TABLE `samples` RENAME COLUMN `title` TO `subject`;
func printRename(rename dbmigrate.Rename) {
	if rename.Column == "" {
		item("%s: %s keeps every column of %s. Left as it is, the plan creates %s empty and the rows stay in %s.",
			rename, rename.To, rename.Table, rename.To, rename.Table)
	} else {
		item("%s: both hold the same definition. Left as it is, the plan drops %s with its data and adds %s empty.",
			rename, rename.Column, rename.To)
	}
	for _, statement := range rename.Statements {
		fmt.Printf("      %s;\n", statement)
	}
}

// answers reads what the user types at the prompts. One reader serves every
// prompt: a reader of its own for each would take, with the answer it was
// asked for, the answers typed ahead for the prompts after it.
var answers = bufio.NewReader(os.Stdin)

// confirm asks question, after lead, and reports whether the answer typed
// is y or yes; any other answer, an empty one included, is a no. The
// question goes to standard error, so it reaches the terminal of a run
// whose output is piped or kept in a file. An answer that does not come from
// a terminal is not echoed on it, so the line the question stands on is
// ended for what is printed next.
func confirm(lead, question string) bool {
	fmt.Fprintf(os.Stderr, "%s%s %s (y/N): ", lead, paint(os.Stderr, colorCyan, symbolPrompt), question)
	input, _ := answers.ReadString('\n')
	if !isTerminal(os.Stdin) {
		fmt.Fprintln(os.Stderr)
	}
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// The lines of this program are shaped the way gg shapes its own, which the
// package doing that, internal to the framework, is out of reach of a
// program written as project code: a section title, the entries under it,
// the line the run ends on with a note under it, in the same symbols and
// the same colors, on the terminal gg found to take them.
const (
	symbolSection = "▶"
	symbolSuccess = "✔"
	symbolError   = "✘"
	symbolItem    = "→"
	symbolPrompt  = "?"

	colorGreen = "92"
	colorRed   = "91"
	colorCyan  = "96"
	colorGrey  = "90"
	colorBold  = "1"
)

// section prints the title of a block of entries.
func section(title string) {
	fmt.Printf("\n%s %s\n", paint(os.Stdout, colorCyan, symbolSection), paint(os.Stdout, colorBold, title))
}

// item prints a plain entry.
func item(format string, args ...any) {
	fmt.Printf("  %s %s\n", paint(os.Stdout, colorGrey, symbolItem), fmt.Sprintf(format, args...))
}

// success prints an entry that went well.
func success(format string, args ...any) {
	fmt.Printf("  %s %s\n", paint(os.Stdout, colorGreen, symbolSuccess), fmt.Sprintf(format, args...))
}

// done prints the line a run that went well ends on.
func done(format string, args ...any) {
	fmt.Printf("\n%s %s\n", paint(os.Stdout, colorGreen, symbolSuccess), fmt.Sprintf(format, args...))
}

// note prints what to do next, under the line the run ends on.
func note(format string, args ...any) {
	fmt.Printf("  %s\n", paint(os.Stdout, colorGrey, fmt.Sprintf(format, args...)))
}

// paint returns text in the color when gg said the terminal takes colors,
// and, for standard error, when it is a terminal itself: a standard error
// redirected to a file carries no escape sequence. The sequence ending the
// text is the one gg ends it with: bold is taken back alone, a color with
// everything else.
func paint(stream *os.File, color, text string) string {
	if !migrateColor || stream == os.Stderr && !isTerminal(stream) {
		return text
	}
	reset := "0"
	if color == colorBold {
		reset = "22"
	}
	return "\x1b[" + color + "m" + text + "\x1b[" + reset + "m"
}

// isTerminal reports whether file is a terminal rather than a pipe or a
// regular file.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// counted writes a number of things the way a sentence does: "1 statement",
// "22 statements".
func counted(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// elapsed formats how long the statements took: 240ms under a second, 1.3s
// under a minute, 1m12s from there on.
func elapsed(d time.Duration) string {
	switch {
	case d < time.Second:
		return d.Round(time.Millisecond).String()
	case d < time.Minute:
		return d.Round(100 * time.Millisecond).String()
	default:
		return d.Round(time.Second).String()
	}
}
