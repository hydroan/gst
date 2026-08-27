package dbmigrate_test

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/dbmigrate"
	"github.com/hydroan/gst/internal/testcontainer"
	"github.com/hydroan/gst/model"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests prepares both databases the migration tests target. They need two at
// once and no server at all, which is what SetupDatabase is for; os.Exit in
// TestMain would skip the deferred releases, hence the wrapper.
func runTests(m *testing.M) int {
	releaseMySQL, err := testcontainer.SetupDatabase(config.DBMySQL)
	if err != nil {
		panic(err)
	}
	defer func() { _ = releaseMySQL() }()

	releasePostgres, err := testcontainer.SetupDatabase(config.DBPostgres)
	if err != nil {
		panic(err)
	}
	defer func() { _ = releasePostgres() }()

	return m.Run()
}

func TestMigrate(t *testing.T) {
	t.Run("mysql", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBMySQL, User{}, Group{}, Sample{})
		require.NoError(t, err)

		database := fmt.Sprintf("gst_dbmigrate_test_%d", time.Now().UnixNano())
		createMySQLDatabase(t, mysqlDatabaseConfig(), database)
		t.Cleanup(func() {
			dropMySQLDatabase(t, mysqlDatabaseConfig(), database)
		})
		databaseConfig := mysqlDatabaseConfig()
		databaseConfig.Database = database

		migrated, _, err := dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.True(t, migrated)

		migrated, _, err = dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
			&dbmigrate.MigrateOption{})
		require.NoError(t, err)
		require.True(t, migrated)

		// A converged schema plans nothing on a re-run, custom indexes included.
		migrated, _, err = dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.False(t, migrated)
	})

	t.Run("postgres", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBPostgres, User{}, Group{}, Sample{})
		require.NoError(t, err)

		database := fmt.Sprintf("gst_dbmigrate_test_%d", time.Now().UnixNano())
		adminConfig := postgresDatabaseConfig(os.Getenv(config.POSTGRES_DATABASE))
		createPostgresDatabase(t, adminConfig, database)
		t.Cleanup(func() {
			dropPostgresDatabase(t, adminConfig, database)
		})
		databaseConfig := postgresDatabaseConfig(database)

		migrated, _, err := dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			},
		)
		require.NoError(t, err)
		require.True(t, migrated)

		migrated, _, err = dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOption{},
		)
		require.NoError(t, err)
		require.True(t, migrated)

		migrated, _, err = dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			},
		)
		require.NoError(t, err)
		require.False(t, migrated)
	})

	t.Run("sqlite", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBSqlite, User{}, Group{}, Sample{})
		require.NoError(t, err)

		database := filepath.Join(t.TempDir(), "test.db")
		migrated, _, err := dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.True(t, migrated)

		migrated, _, err = dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOption{})
		require.NoError(t, err)
		require.True(t, migrated)

		migrated, _, err = dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.False(t, migrated)
	})
}

// TestMigrateDropsRemovedIndex pins the planner's drop path for a secondary
// index that disappears from the desired schema: the index survives without
// EnableDrop, and is planned and executed as a drop with it. The fixture
// index comes from a struct tag because that is the shape a base-struct tag
// removal produces on every table at once, and tag indexes flow through the
// same desired-DDL rendering as everything else the dumper emits.
func TestMigrateDropsRemovedIndex(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	withIndex, err := dumper.Dump(config.DBMySQL, &TagIndexedWidget{})
	require.NoError(t, err)
	require.Contains(t, withIndex, "`idx_widgets_tag`")
	withoutIndex, err := dumper.Dump(config.DBMySQL, &PlainWidget{})
	require.NoError(t, err)
	require.NotContains(t, withoutIndex, "idx_widgets_tag")

	database := fmt.Sprintf("gst_dbmigrate_dropindex_%d", time.Now().UnixNano())
	createMySQLDatabase(t, mysqlDatabaseConfig(), database)
	t.Cleanup(func() {
		dropMySQLDatabase(t, mysqlDatabaseConfig(), database)
	})
	databaseConfig := mysqlDatabaseConfig()
	databaseConfig.Database = database

	migrated, _, err := dbmigrate.Migrate([]string{withIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Equal(t, 1, mysqlIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// Without EnableDrop the index survives: the plan still reports the
	// change, but renders the destructive statement as skipped instead of
	// executing it.
	migrated, _, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Equal(t, 1, mysqlIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// With EnableDrop the removal is planned...
	migrated, _, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)

	// ...and applying it drops the index for good.
	migrated, _, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Equal(t, 0, mysqlIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// The converged schema plans nothing on a re-run.
	migrated, _, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.False(t, migrated)
}

// TestMigrateDropsRemovedIndexOnPostgres pins the same drop path on
// postgres, where the shape differs from mysql on both sides: the dumper
// renders a tag index as a standalone CREATE INDEX statement, and the plan
// drops it with postgres DDL.
func TestMigrateDropsRemovedIndexOnPostgres(t *testing.T) {
	dumper, err := dbmigrate.NewSchemaDumper()
	require.NoError(t, err)
	defer dumper.Close()

	withIndex, err := dumper.Dump(config.DBPostgres, &TagIndexedWidget{})
	require.NoError(t, err)
	require.Contains(t, withIndex, "idx_widgets_tag")
	withoutIndex, err := dumper.Dump(config.DBPostgres, &PlainWidget{})
	require.NoError(t, err)
	require.NotContains(t, withoutIndex, "idx_widgets_tag")

	database := fmt.Sprintf("gst_dbmigrate_dropindex_pg_%d", time.Now().UnixNano())
	adminConfig := postgresDatabaseConfig(os.Getenv(config.POSTGRES_DATABASE))
	createPostgresDatabase(t, adminConfig, database)
	t.Cleanup(func() {
		dropPostgresDatabase(t, adminConfig, database)
	})
	databaseConfig := postgresDatabaseConfig(database)

	migrated, _, err := dbmigrate.Migrate([]string{withIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Equal(t, 1, postgresIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// Without EnableDrop the index survives: the plan still reports the
	// change, but renders the destructive statement as skipped instead of
	// executing it.
	migrated, _, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Equal(t, 1, postgresIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// With EnableDrop the removal is planned...
	migrated, _, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)

	// ...and applying it drops the index for good.
	migrated, _, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Equal(t, 0, postgresIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// The converged schema plans nothing on a re-run.
	migrated, _, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.False(t, migrated)
}

func TestMigrateTableRenameAdvisory(t *testing.T) {
	database := fmt.Sprintf("gst_dbmigrate_rename_%d", time.Now().UnixNano())
	createMySQLDatabase(t, mysqlDatabaseConfig(), database)
	t.Cleanup(func() {
		dropMySQLDatabase(t, mysqlDatabaseConfig(), database)
	})
	databaseConfig := mysqlDatabaseConfig()
	databaseConfig.Database = database

	before := "CREATE TABLE `samples` (\n" +
		"  `id` char(36) NOT NULL,\n" +
		"  `code` varchar(64) NOT NULL,\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  INDEX `idx_samples_code` (`code`)\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
	after := strings.ReplaceAll(before, "samples", "records")

	migrated, advisory, err := dbmigrate.Migrate([]string{before}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Empty(t, advisory)

	// The plan for the renamed model drops `samples` and creates `records`;
	// the advisory must offer the metadata-only statements instead.
	migrated, advisory, err = dbmigrate.Migrate([]string{after}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Contains(t, advisory, "RENAME TABLE `samples` TO `records`;")
	require.Contains(t, advisory, "ALTER TABLE `records` RENAME INDEX `idx_samples_code` TO `idx_records_code`;")

	// Applying the advisory instead of the plan leaves nothing to migrate.
	execMySQL(t, databaseConfig, "RENAME TABLE `samples` TO `records`")
	execMySQL(t, databaseConfig, "ALTER TABLE `records` RENAME INDEX `idx_samples_code` TO `idx_records_code`")
	migrated, advisory, err = dbmigrate.Migrate([]string{after}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.False(t, migrated)
	require.Empty(t, advisory)
}

func TestMigrateTableRenameAdvisoryWithRemainingChanges(t *testing.T) {
	database := fmt.Sprintf("gst_dbmigrate_rename_drift_%d", time.Now().UnixNano())
	createMySQLDatabase(t, mysqlDatabaseConfig(), database)
	t.Cleanup(func() {
		dropMySQLDatabase(t, mysqlDatabaseConfig(), database)
	})
	databaseConfig := mysqlDatabaseConfig()
	databaseConfig.Database = database

	before := "CREATE TABLE `samples` (\n" +
		"  `id` char(36) NOT NULL,\n" +
		"  `code` varchar(64) NOT NULL,\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  INDEX `idx_samples_code` (`code`)\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"

	migrated, advisory, err := dbmigrate.Migrate([]string{before}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Empty(t, advisory)

	// The model renames the table and adds a column in the same step, so the
	// created table is a column superset of the dropped one. The advisory must
	// still offer the rename and list the addition as a remaining change.
	after := strings.ReplaceAll(
		strings.Replace(before, "  `code` varchar(64) NOT NULL,\n",
			"  `code` varchar(64) NOT NULL,\n  `remark` varchar(255) NOT NULL DEFAULT '',\n", 1),
		"samples", "records",
	)
	migrated, advisory, err = dbmigrate.Migrate([]string{after}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Contains(t, advisory, "RENAME TABLE `samples` TO `records`;")
	require.Contains(t, advisory, "ALTER TABLE `records` RENAME INDEX `idx_samples_code` TO `idx_records_code`;")
	require.Contains(t, advisory, "remaining change: ALTER TABLE `records` ADD COLUMN `remark`")

	// After the rename, only the remaining column addition is left in the
	// plan, and there is no drop/create pair left to advise about.
	execMySQL(t, databaseConfig, "RENAME TABLE `samples` TO `records`")
	execMySQL(t, databaseConfig, "ALTER TABLE `records` RENAME INDEX `idx_samples_code` TO `idx_records_code`")
	migrated, advisory, err = dbmigrate.Migrate([]string{after}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Empty(t, advisory)
}

func TestMigrateTableRenameAdvisoryOnPostgres(t *testing.T) {
	database := fmt.Sprintf("gst_dbmigrate_rename_pg_%d", time.Now().UnixNano())
	adminConfig := postgresDatabaseConfig(os.Getenv(config.POSTGRES_DATABASE))
	createPostgresDatabase(t, adminConfig, database)
	t.Cleanup(func() {
		dropPostgresDatabase(t, adminConfig, database)
	})
	databaseConfig := postgresDatabaseConfig(database)

	before := "CREATE TABLE samples (\n" +
		"  id char(36) NOT NULL,\n" +
		"  code varchar(64) NOT NULL,\n" +
		"  PRIMARY KEY (id)\n" +
		");\n" +
		"CREATE INDEX idx_samples_code ON samples (code);"
	after := strings.ReplaceAll(before, "samples", "records")

	migrated, advisory, err := dbmigrate.Migrate([]string{before}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Empty(t, advisory)

	// The plan for the renamed model drops "samples" and creates "records";
	// the advisory must offer the metadata-only statements in postgres syntax.
	migrated, advisory, err = dbmigrate.Migrate([]string{after}, config.DBPostgres, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Contains(t, advisory, `ALTER TABLE "samples" RENAME TO "records";`)
	require.Contains(t, advisory, `ALTER INDEX "idx_samples_code" RENAME TO "idx_records_code";`)

	// Applying the advisory instead of the plan leaves nothing to migrate.
	execPostgres(t, databaseConfig, `ALTER TABLE "samples" RENAME TO "records"`)
	execPostgres(t, databaseConfig, `ALTER INDEX "idx_samples_code" RENAME TO "idx_records_code"`)
	migrated, advisory, err = dbmigrate.Migrate([]string{after}, config.DBPostgres, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.False(t, migrated)
	require.Empty(t, advisory)
}

func TestMigrateTableRenameAdvisoryWithRemainingChangesOnPostgres(t *testing.T) {
	database := fmt.Sprintf("gst_dbmigrate_rename_pg_drift_%d", time.Now().UnixNano())
	adminConfig := postgresDatabaseConfig(os.Getenv(config.POSTGRES_DATABASE))
	createPostgresDatabase(t, adminConfig, database)
	t.Cleanup(func() {
		dropPostgresDatabase(t, adminConfig, database)
	})
	databaseConfig := postgresDatabaseConfig(database)

	before := "CREATE TABLE samples (\n" +
		"  id char(36) NOT NULL,\n" +
		"  code varchar(64) NOT NULL,\n" +
		"  PRIMARY KEY (id)\n" +
		");\n" +
		"CREATE INDEX idx_samples_code ON samples (code);"

	migrated, advisory, err := dbmigrate.Migrate([]string{before}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Empty(t, advisory)

	// The model renames the table and adds a column in the same step, so the
	// created table is a column superset of the dropped one. The advisory must
	// still offer the rename and list the addition as a remaining change.
	after := strings.ReplaceAll(
		strings.Replace(before, "  code varchar(64) NOT NULL,\n",
			"  code varchar(64) NOT NULL,\n  remark varchar(255) NOT NULL DEFAULT '',\n", 1),
		"samples", "records",
	)
	migrated, advisory, err = dbmigrate.Migrate([]string{after}, config.DBPostgres, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Contains(t, advisory, `ALTER TABLE "samples" RENAME TO "records";`)
	require.Contains(t, advisory, `ALTER INDEX "idx_samples_code" RENAME TO "idx_records_code";`)
	require.Contains(t, advisory, "remaining change:")
	require.Contains(t, advisory, "ADD COLUMN")

	// After the rename, only the remaining column addition is left in the
	// plan, and there is no drop/create pair left to advise about.
	execPostgres(t, databaseConfig, `ALTER TABLE "samples" RENAME TO "records"`)
	execPostgres(t, databaseConfig, `ALTER INDEX "idx_samples_code" RENAME TO "idx_records_code"`)
	migrated, advisory, err = dbmigrate.Migrate([]string{after}, config.DBPostgres, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, migrated)
	require.Empty(t, advisory)
}

// newDatabaseConfig reads back the connection the test container was prepared on.
func newDatabaseConfig(hostKey, portKey, userKey, passwordKey, database string) *dbmigrate.DatabaseConfig {
	port, err := strconv.Atoi(os.Getenv(portKey))
	if err != nil {
		panic(err)
	}
	return &dbmigrate.DatabaseConfig{
		Host:     os.Getenv(hostKey),
		Port:     port,
		Username: os.Getenv(userKey),
		Password: os.Getenv(passwordKey),
		Database: database,
	}
}

func mysqlDatabaseConfig() *dbmigrate.DatabaseConfig {
	return newDatabaseConfig(config.MYSQL_HOST, config.MYSQL_PORT, config.MYSQL_USERNAME, config.MYSQL_PASSWORD,
		os.Getenv(config.MYSQL_DATABASE))
}

func createMySQLDatabase(t *testing.T, cfg *dbmigrate.DatabaseConfig, database string) {
	t.Helper()
	execMySQL(t, cfg, "CREATE DATABASE "+database)
}

func dropMySQLDatabase(t *testing.T, cfg *dbmigrate.DatabaseConfig, database string) {
	t.Helper()

	db, err := sql.Open("mysql", mysqlDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, _ = db.Exec("DROP DATABASE IF EXISTS " + database)
}

// execMySQL runs one statement on the configured MySQL database. The driver
// is registered by the sqldef mysql package that dbmigrate itself imports.
func execMySQL(t *testing.T, cfg *dbmigrate.DatabaseConfig, statement string) {
	t.Helper()

	db, err := sql.Open("mysql", mysqlDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(statement)
	require.NoError(t, err)
}

func mysqlDSN(cfg *dbmigrate.DatabaseConfig) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
}

// mysqlIndexCount reports whether the named index exists on one table, as the
// number of matching index names information_schema holds: 1 when present, 0
// when dropped.
func mysqlIndexCount(t *testing.T, cfg *dbmigrate.DatabaseConfig, table, index string) int {
	t.Helper()

	db, err := sql.Open("mysql", mysqlDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics WHERE table_schema = ? AND table_name = ? AND index_name = ?",
		cfg.Database, table, index,
	).Scan(&count))
	return count
}

// postgresIndexCount reports whether the named index exists on one table, as
// the number of matching index names pg_indexes holds: 1 when present, 0
// when dropped.
func postgresIndexCount(t *testing.T, cfg *dbmigrate.DatabaseConfig, table, index string) int {
	t.Helper()

	db, err := sql.Open("postgres", postgresDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM pg_indexes WHERE schemaname = 'public' AND tablename = $1 AND indexname = $2",
		table, index,
	).Scan(&count))
	return count
}

// TagIndexedWidget and PlainWidget share one table: the second is the first
// with its struct-tag index removed, which is the change
// TestMigrateDropsRemovedIndex drives through the planner.
type TagIndexedWidget struct {
	Tag string `json:"tag" gorm:"size:191;index"`

	model.Base
}

func (*TagIndexedWidget) TableName() string { return "widgets" }

type PlainWidget struct {
	Tag string `json:"tag" gorm:"size:191"`

	model.Base
}

func (*PlainWidget) TableName() string { return "widgets" }

func postgresDatabaseConfig(database string) *dbmigrate.DatabaseConfig {
	cfg := newDatabaseConfig(config.POSTGRES_HOST, config.POSTGRES_PORT, config.POSTGRES_USERNAME, config.POSTGRES_PASSWORD, database)
	cfg.SSLMode = "disable"
	return cfg
}

func createPostgresDatabase(t *testing.T, cfg *dbmigrate.DatabaseConfig, database string) {
	t.Helper()

	db, err := sql.Open("postgres", postgresDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("CREATE DATABASE " + database)
	require.NoError(t, err)
}

func dropPostgresDatabase(t *testing.T, cfg *dbmigrate.DatabaseConfig, database string) {
	t.Helper()

	db, err := sql.Open("postgres", postgresDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, _ = db.Exec("DROP DATABASE IF EXISTS " + database)
}

// execPostgres runs one statement on the configured PostgreSQL database. The
// driver is registered by the sqldef postgres package that dbmigrate itself
// imports.
func execPostgres(t *testing.T, cfg *dbmigrate.DatabaseConfig, statement string) {
	t.Helper()

	db, err := sql.Open("postgres", postgresDSN(cfg))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(statement)
	require.NoError(t, err)
}

func postgresDSN(cfg *dbmigrate.DatabaseConfig) string {
	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.Username, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   "/" + cfg.Database,
	}

	options := url.Values{}
	if cfg.SSLMode != "" {
		options.Set("sslmode", cfg.SSLMode)
	}
	dsn.RawQuery = options.Encode()

	return dsn.String()
}
