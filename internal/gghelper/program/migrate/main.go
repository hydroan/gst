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

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dbmigrate"
	"github.com/hydroan/gst/model"

	// bootstrap links every framework package the running service links, so
	// this program sees the same registrations, and brings up the router and
	// the modules the way the service does.
	"github.com/hydroan/gst/bootstrap"
)

// The flags gg passes: the project's module path, the mode, and for a
// schema dump of part of the models its source and the Go files under it, as
// gg listed them.
var (
	migrateModule            string
	migrateDryRun            bool
	migrateYes               bool
	migrateSchemaOnly        bool
	migrateSchemaSource      string
	migrateSchemaSourceFiles fileList
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
	flag.BoolVar(&migrateSchemaOnly, "schema-only", false, "dump the schema and stop")
	flag.StringVar(&migrateSchemaSource, "schema-source", "", "the model source the schema dump covers; empty covers every registered model")
	flag.Var(&migrateSchemaSourceFiles, "schema-source-file", "a Go file under schema-source; repeat for each")
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
		fmt.Println("\n▶ Result")
		fmt.Println("  → Nothing to migrate: the application creates the tables of its in-memory database from the models every time it starts.")
		fmt.Println("  → To migrate a database file, set sqlite.is_memory = false and point sqlite.path at the file.")
		return
	}

	// Perform migration.
	if err := performMigration(schema, dbConfig); err != nil {
		exitWithError(err)
	}
}

// exitWithError prints a command-style error and exits without a panic stack.
func exitWithError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
	fmt.Println("\n▶ Model Schema")
	fmt.Printf("  → Database: %s\n", config.App.Database.Type)
	if strings.TrimSpace(source) != "" {
		fmt.Printf("  → Source: %s\n", source)
	}
	fmt.Printf("  → Matched models: %d\n\n", modelCount)
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
	fmt.Println("\n▶ Database Migration")
	fmt.Printf("  → Database: %s\n", config.App.Database.Type)
	fmt.Printf("  → Target: %s\n", databaseTarget(cfg))
	fmt.Printf("  → Registered models: %d\n", modelCount)
	fmt.Printf("  ✔ Schema written: %s\n", schemaFile)
	switch {
	case migrateDryRun:
		fmt.Println("  → Mode: dry run")
	case migrateYes:
		fmt.Println("  → Mode: apply without prompt")
	default:
		fmt.Println("  → Mode: confirm before apply")
	}
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

// performMigration executes the migration process: dry run, confirmation, and actual execution.
func performMigration(schema string, cfg *dbmigrate.DatabaseConfig) error {
	dbtyp := config.App.Database.Type

	fmt.Println("\n▶ Migration Plan")

	// Planned once, against the schema the database has now. What is applied
	// below is this plan, statement for statement: planning again to execute
	// would plan against whatever the database has by then, which is not what
	// was shown and approved.
	plan, err := dbmigrate.Migrate([]string{schema}, dbtyp, cfg, &dbmigrate.MigrateOption{
		DryRun:     true,
		EnableDrop: true,
	})
	if err != nil {
		return err
	}

	if !plan.Changed() {
		fmt.Println("  → No changes detected.")
		return nil
	}

	if plan.CreatesDatabase {
		fmt.Println("  → The target database file does not exist yet: the plan starts from an empty database, and applying it creates the file.")
	}

	// The advisory gets its own section after the plan, so suspected table
	// and index renames stay visible right before the reviewer decides.
	if len(plan.Advisory) != 0 {
		fmt.Println("\n▶ Rename Advisory")
		fmt.Print(plan.Advisory)
	}

	if migrateDryRun {
		fmt.Println("\n▶ Result")
		fmt.Println("  → Dry run completed. No changes were applied.")
		return nil
	}

	// Confirm execution with the user.
	if !migrateYes && !confirmExecution() {
		fmt.Println("  → Migration canceled.")
		return nil
	}

	fmt.Println("\n▶ Apply Migration")

	// The statements shown above, as they stand.
	if err := dbmigrate.Apply(plan, dbtyp, cfg); err != nil {
		return err
	}
	fmt.Println("  ✔ Migration executed successfully.")
	return nil
}

// confirmExecution prompts the user for confirmation to proceed.
func confirmExecution() bool {
	fmt.Print("\n? Apply the migration to the target database? Type \"yes\" to continue: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	return strings.ToLower(input) == "yes"
}
