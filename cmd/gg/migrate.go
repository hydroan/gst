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

var (
	migrateDryRun bool
	migrateYes    bool
)

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Preview migration SQL without applying changes")
	migrateCmd.Flags().BoolVar(&migrateYes, "yes", false, "Apply migration without prompting for confirmation")
}

func buildMigrateProgram(moduleName string) string {
	content := migrateTemplate
	content = strings.ReplaceAll(content, "{{MODULE}}", moduleName)
	content = strings.ReplaceAll(content, "{{DRY_RUN}}", strconv.FormatBool(migrateDryRun))
	content = strings.ReplaceAll(content, "{{YES}}", strconv.FormatBool(migrateYes))
	return fmt.Sprintf("%s\n%s", consts.CodeGeneratedComment(), content)
}

func runMigrateProgram(content string) error {
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

	clioutput.Section("Migration")
	clioutput.Info("", "Preparing migration...")

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
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "{{MODULE}}/model"
	_ "{{MODULE}}/module"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/middleware"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/module"
	"github.com/hydroan/gst/pkg/dbmigrate"
	"github.com/hydroan/gst/router"
)

const migrateDryRun = {{DRY_RUN}}
const migrateYes = {{YES}}

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

	// Dump the schema for the collected models.
	schema, err := dumpSchema(models)
	if err != nil {
		exitWithError(err)
	}

	// Get database configuration based on the configured database type.
	dbConfig := getDatabaseConfig()

	// Write the schema to a generated file for reference or debugging.
	schemaFile, err := writeSchemaFile(schema)
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

// collectModels collects all models registered in model.TableChan.
func collectModels() []any {
	models := make([]any, 0)
	maxCount := len(model.TableChan)
	if maxCount == 0 {
		return models
	}
	count := 0
	for m := range model.TableChan {
		models = append(models, m)
		count++
		if count >= maxCount {
			break
		}
	}
	return models
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

// writeSchemaFile writes the generated schema under the database-specific migration directory.
func writeSchemaFile(schema string) (string, error) {
	schemaDir := filepath.Join("generated", "migrate", string(config.App.Database.Type))
	schemaFile := filepath.Join(schemaDir, "schema.sql")
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create %s directory: %w", schemaDir, err)
	}
	if err := os.WriteFile(schemaFile, []byte(schema), 0o644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", schemaFile, err)
	}
	return schemaFile, nil
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
	hasChange, err := dbmigrate.Migrate([]string{schema}, dbtyp, cfg, &dbmigrate.MigrateOption{
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
	_, err = dbmigrate.Migrate([]string{schema}, dbtyp, cfg, &dbmigrate.MigrateOption{
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
