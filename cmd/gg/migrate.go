package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/types/consts"
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
		moduleName, err := gen.GetModulePath()
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
		moduleName, err := gen.GetModulePath()
		if err != nil {
			return fmt.Errorf("failed to get module path: %w", err)
		}

		source := ""
		if len(args) > 0 {
			source = args[0]
		}
		return runMigrateSchemaProgram(buildMigrateSchemaProgram(moduleName, source))
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
	return buildMigrateProgramForMode(moduleName, false, "")
}

func buildMigrateSchemaProgram(moduleName string, source string) string {
	return buildMigrateProgramForMode(moduleName, true, source)
}

func buildMigrateProgramForMode(moduleName string, schemaOnly bool, schemaSource string) string {
	content := migrateTemplate
	content = strings.ReplaceAll(content, "{{MODULE}}", moduleName)
	content = strings.ReplaceAll(content, "{{DRY_RUN}}", strconv.FormatBool(migrateDryRun))
	content = strings.ReplaceAll(content, "{{YES}}", strconv.FormatBool(migrateYes))
	content = strings.ReplaceAll(content, "{{SCHEMA_ONLY}}", strconv.FormatBool(schemaOnly))
	content = strings.ReplaceAll(content, "{{SCHEMA_SOURCE}}", strconv.Quote(schemaSource))
	return fmt.Sprintf("%s\n%s", consts.CodeGeneratedComment(), content)
}

func runMigrateProgram(content string) error {
	return runGeneratedMigrateProgram(content, "Migration", "Preparing migration...")
}

func runMigrateSchemaProgram(content string) error {
	return runGeneratedMigrateProgram(content, "Migration Schema", "Preparing schema dump...")
}

func runGeneratedMigrateProgram(content string, section string, message string) error {
	tempDir, err := os.MkdirTemp("", "gg-migrate-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary migration directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	runnerFile := filepath.Join(tempDir, "main.go")
	if err = os.WriteFile(runnerFile, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write temporary migration program: %w", err)
	}

	modFile := filepath.Join(tempDir, "migrate.mod")
	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("failed to read go.mod: %w", err)
	}
	// #nosec G703 -- modFile is created under an os.MkdirTemp-owned directory.
	if err = os.WriteFile(modFile, goMod, 0o600); err != nil {
		return fmt.Errorf("failed to write temporary migrate.mod: %w", err)
	}

	goSum, err := os.ReadFile("go.sum")
	if os.IsNotExist(err) {
		goSum = nil
	} else if err != nil {
		return fmt.Errorf("failed to read go.sum: %w", err)
	}
	if goSum != nil {
		sumFile := filepath.Join(tempDir, "migrate.sum")
		// #nosec G703 -- sumFile is created under an os.MkdirTemp-owned directory.
		if err = os.WriteFile(sumFile, goSum, 0o600); err != nil {
			return fmt.Errorf("failed to write temporary migrate.sum: %w", err)
		}
	}

	clioutput.Section(section)
	clioutput.Info("", "%s", message)

	runCmd := exec.Command("go", "run", "-mod=mod", "-modfile", modFile, runnerFile)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	runCmd.Stdin = os.Stdin
	if err = runCmd.Run(); err != nil {
		return fmt.Errorf("failed to run migration: %w", err)
	}
	return nil
}

const migrateTemplate = `package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	_ "{{MODULE}}/model"
	_ "{{MODULE}}/module"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/dbmigrate"
	"github.com/hydroan/gst/router"
)

const migrateDryRun = {{DRY_RUN}}
const migrateYes = {{YES}}
const migrateModule = "{{MODULE}}"
const migrateSchemaOnly = {{SCHEMA_ONLY}}
const migrateSchemaSource = {{SCHEMA_SOURCE}}

func main() {
	// Initialize system components and suppress stdout during initialization
	// to avoid cluttering the migration output.
	initComponents()
	// Ensure config resources are cleaned up when the program exits.
	defer config.Clean()

	// Wait for all models to be registered.
	// This sleep is necessary because some models might be registered asynchronously
	// or during the initialization phase of modules.
	time.Sleep(1 * time.Second)

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

// initComponents initializes the application components (config, middleware, router, module).
// It temporarily suppresses stdout to prevent initialization logs from appearing in the console.
func initComponents() {
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
	if err = middleware.Init(); err != nil {
		exitWithError(err)
	}
	if err = router.Init(); err != nil {
		exitWithError(err)
	}
	if err = module.Init(); err != nil {
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
	models, err = filterModelsBySource(models, migrateSchemaSource)
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

func filterModelsBySource(models []any, source string) ([]any, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return models, nil
	}

	targetTypes, err := collectSourceModelTypes(source)
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

func collectSourceModelTypes(source string) (map[modelTypeKey]struct{}, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect model source %s: %w", source, err)
	}

	var files []string
	if info.IsDir() {
		err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", "generated", "vendor":
					return filepath.SkipDir
				}
				return nil
			}
			if isGoModelSource(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk model source %s: %w", source, err)
		}
	} else {
		if !isGoModelSource(source) {
			return nil, fmt.Errorf("model source must be a Go file or directory: %s", source)
		}
		files = append(files, source)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no Go model files found under %s", source)
	}

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

func isGoModelSource(path string) bool {
	return strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
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

	// Dry Run: Check for changes without executing.
	hasChange, advisory, err := dbmigrate.Migrate([]string{schema}, dbtyp, cfg, &dbmigrate.MigrateOption{
		DryRun:     true,
		EnableDrop: true,
	})
	if err != nil {
		return err
	}

	if !hasChange {
		fmt.Println("  → No changes detected.")
		return nil
	}

	// The advisory gets its own section after the plan, so suspected index
	// renames stay visible right before the reviewer decides.
	if len(advisory) != 0 {
		fmt.Println("\n▶ Index Rename Advisory")
		fmt.Print(advisory)
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

	// Execute Migration.
	_, _, err = dbmigrate.Migrate([]string{schema}, dbtyp, cfg, &dbmigrate.MigrateOption{
		DryRun:     false,
		EnableDrop: true,
	})
	if err != nil {
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
