package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:           "migrate",
	Short:         "Run database migrations",
	Long:          "Generate and execute database migration code based on current models",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Get module name
		moduleName, err := gghelper.ModulePath()
		if err != nil {
			return fmt.Errorf("failed to get module path: %w", err)
		}

		return runMigrateProgram(buildMigrateProgram(moduleName))
	},
}

var migrateSchemaCmd = &cobra.Command{
	Use:           "schema [path]",
	Short:         "Print schema SQL for registered models",
	Long:          "Print target schema SQL for all registered models, or only registered models declared in a file or directory",
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		moduleName, err := gghelper.ModulePath()
		if err != nil {
			return fmt.Errorf("failed to get module path: %w", err)
		}

		source := ""
		if len(args) > 0 {
			source = strings.TrimSpace(args[0])
		}
		files, err := migrateSourceFiles(source)
		if err != nil {
			return err
		}
		return runMigrateSchemaProgram(buildMigrateSchemaProgram(moduleName, source, files))
	},
}

var (
	migrateDryRun bool
	migrateYes    bool
)

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Preview migration SQL without applying changes")
	migrateCmd.Flags().BoolVar(&migrateYes, "yes", false, "Apply migration without prompting for confirmation")
	migrateCmd.AddCommand(migrateSchemaCmd)
}

func buildMigrateProgram(moduleName string) string {
	return buildMigrateProgramForMode(moduleName, false, "", nil)
}

func buildMigrateSchemaProgram(moduleName string, source string, sourceFiles []string) string {
	return buildMigrateProgramForMode(moduleName, true, source, sourceFiles)
}

func buildMigrateProgramForMode(moduleName string, schemaOnly bool, schemaSource string, schemaSourceFiles []string) string {
	quotedFiles := make([]string, len(schemaSourceFiles))
	for i, file := range schemaSourceFiles {
		quotedFiles[i] = strconv.Quote(file)
	}
	content := migrateTemplate
	content = strings.ReplaceAll(content, "{{PROJECT_IMPORTS}}", migrateProjectImports(moduleName))
	content = strings.ReplaceAll(content, "{{MODULE}}", moduleName)
	content = strings.ReplaceAll(content, "{{DRY_RUN}}", strconv.FormatBool(migrateDryRun))
	content = strings.ReplaceAll(content, "{{YES}}", strconv.FormatBool(migrateYes))
	content = strings.ReplaceAll(content, "{{SCHEMA_ONLY}}", strconv.FormatBool(schemaOnly))
	content = strings.ReplaceAll(content, "{{SCHEMA_SOURCE}}", strconv.Quote(schemaSource))
	content = strings.ReplaceAll(content, "{{SCHEMA_SOURCE_FILES}}", "[]string{"+strings.Join(quotedFiles, ", ")+"}")
	return fmt.Sprintf("%s\n\n%s", consts.CodeGeneratedComment(), content)
}

// migrateProjectImports renders the project packages the migration program
// links, one blank import per line: the same list a generated main.go
// imports, so the program registers every model the running service does.
//
// A package the project does not have yet — a scaffold gg gen has not
// restored since the framework grew one — is left out, with a note: it
// registers nothing, and the service does not build without it either, so
// the migration must not fail on its account. Migration never writes source
// files; restoring the scaffold is gen's job.
func migrateProjectImports(moduleName string) string {
	lines := make([]string, 0, len(ggconst.ProjectImportDirs))
	for _, dir := range ggconst.ProjectImportDirs {
		if !hasGoSources(dir) {
			clioutput.Warn("", "%s/ has no Go files and is left out of the migration program; run gg gen to restore the scaffold", dir)
			continue
		}
		lines = append(lines, fmt.Sprintf("\t_ %q", moduleName+"/"+dir))
	}
	return strings.Join(lines, "\n")
}

// hasGoSources reports whether dir holds a Go source file that is not a
// test: a directory of test files alone is no package to import.
func hasGoSources(dir string) bool {
	sources, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	for _, source := range sources {
		if !strings.HasSuffix(source, "_test.go") {
			return true
		}
	}
	return false
}

func runMigrateProgram(content string) error {
	return runGeneratedMigrateProgram(content, "Migration", "Preparing migration...")
}

// migrateSourceFiles lists the Go files gg migrate schema reads model types
// from: source itself when it names a Go file, or else the Go files below it
// that are not tests, walked by the rules a walk over the project's code
// follows (gghelper.ExcludedDir). For model it lists model/record.go and
// model/sample/item.go, and leaves out model/record_test.go and every file of
// model/testdata. An empty source lists nothing: the schema covers every
// registered model.
func migrateSourceFiles(source string) ([]string, error) {
	if source == "" {
		return nil, nil
	}
	isSource := func(path string) bool {
		return strings.HasSuffix(path, ggconst.ExtensionGo) && !strings.HasSuffix(path, ggconst.PatternTestFile)
	}
	info, err := os.Stat(source)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to inspect model source %s", source)
	}
	if !info.IsDir() {
		if !isSource(source) {
			return nil, errors.Newf("model source must be a Go file or directory: %s", source)
		}
		return []string{source}, nil
	}

	var files []string
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if gghelper.ExcludedDir(source, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if isSource(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to walk model source %s", source)
	}
	if len(files) == 0 {
		return nil, errors.Newf("no Go model files found under %s", source)
	}
	sort.Strings(files)
	return files, nil
}

func runMigrateSchemaProgram(content string) error {
	return runGeneratedMigrateProgram(content, "Migration Schema", "Preparing schema dump...")
}

func runGeneratedMigrateProgram(content string, section string, message string) error {
	clioutput.Section(section)
	clioutput.Info("", "%s", message)

	// Migration prints its own progress and prompts for confirmation, so its
	// output and input stay connected to the terminal.
	return gghelper.ProjectProgram{Content: content, Stdout: os.Stdout, Interactive: true}.Run()
}

const migrateTemplate = `package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"

{{PROJECT_IMPORTS}}

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dbmigrate"
	"github.com/hydroan/gst/model"

	// bootstrap links every framework package the running service links, so
	// this program sees the same registrations, and brings up the router and
	// the modules the way the service does.
	"github.com/hydroan/gst/bootstrap"
)

const migrateDryRun = {{DRY_RUN}}
const migrateYes = {{YES}}
const migrateModule = "{{MODULE}}"
const migrateSchemaOnly = {{SCHEMA_ONLY}}
const migrateSchemaSource = {{SCHEMA_SOURCE}}

// migrateSchemaSourceFiles are the Go files under migrateSchemaSource, as gg
// listed them.
var migrateSchemaSourceFiles = {{SCHEMA_SOURCE_FILES}}

func main() {
	// Load the configuration and initialize the router and modules, with
	// stdout suppressed during initialization to avoid cluttering the
	// migration output.
	initConfigRouterAndModules()
	// Ensure config resources are cleaned up when the program exits.
	defer config.Clean()

	// Collect all registered models.
	models := collectModels()

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

	// Get database configuration based on the configured database type.
	dbConfig := getDatabaseConfig()

	// Write the schema to a generated file for reference or debugging.
	schemaFile, err := writeSchemaFile(schema, len(models))
	if err != nil {
		exitWithError(err)
	}

	printMigrationSummary(schemaFile, dbConfig, len(models))

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

// collectModels collects snapshots of models registered through model.Register.
func collectModels() []any {
	return deduplicateModels(model.RegisteredModels())
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
	models, err = filterModelsBySource(models, migrateSchemaSource, migrateSchemaSourceFiles)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("no registered models found")
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
		return nil, fmt.Errorf("no registered models found under %s", source)
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
		return nil, fmt.Errorf("no model type declarations found under %s", source)
	}
	return targetTypes, nil
}

func collectFileModelTypes(filename string, targetTypes map[modelTypeKey]struct{}) error {
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		return fmt.Errorf("failed to parse model source %s: %w", filename, err)
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
		return "", fmt.Errorf("model source %s is outside project root", filename)
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
		return "", fmt.Errorf("failed to create %s directory: %w", schemaDir, err)
	}
	if err := os.WriteFile(schemaFile, []byte(schemaSnapshotContent(schema, modelCount)), 0o644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", schemaFile, err)
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

// getDatabaseConfig constructs the database configuration based on the application config.
func getDatabaseConfig() *dbmigrate.DatabaseConfig {
	var cfg *dbmigrate.DatabaseConfig
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
		cfg = &dbmigrate.DatabaseConfig{
			Database: config.App.Sqlite.Database,
		}
	default:
		exitWithError(fmt.Errorf("unsupported database type: %s", config.App.Database.Type))
	}
	return cfg
}

// printMigrationSummary prints the target and generated schema path before any SQL is applied.
func printMigrationSummary(schemaFile string, cfg *dbmigrate.DatabaseConfig, modelCount int) {
	fmt.Println("\n▶ Database Migration")
	fmt.Printf("  → Database: %s\n", config.App.Database.Type)
	fmt.Printf("  → Target: %s\n", databaseTarget(cfg))
	fmt.Printf("  → Registered models: %d\n", modelCount)
	fmt.Printf("  ✔ Schema written: %s\n", schemaFile)
	if migrateDryRun {
		fmt.Println("  → Mode: dry run")
	} else if migrateYes {
		fmt.Println("  → Mode: apply without prompt")
	} else {
		fmt.Println("  → Mode: confirm before apply")
	}
}

// databaseTarget formats the configured database target without printing credentials.
func databaseTarget(cfg *dbmigrate.DatabaseConfig) string {
	switch config.App.Database.Type {
	case config.DBSqlite:
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
`
