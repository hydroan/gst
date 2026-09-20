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
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests prepares both databases the migration tests target. They need two at
// once and no server at all, which is what SetupDatabase is for; os.Exit in
// TestMain would skip the deferred releases, hence the wrapper.
func runTests(m *testing.M) int {
	releaseMySQL, _, err := testcontainer.SetupDatabase(config.DBMySQL)
	if err != nil {
		panic(err)
	}
	defer func() { _ = releaseMySQL() }()

	releasePostgres, _, err := testcontainer.SetupDatabase(config.DBPostgres)
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
		schema, err := dumper.Dump(config.DBMySQL, User{}, Group{}, Sample{}, DefaultedRecord{})
		require.NoError(t, err)

		database := fmt.Sprintf("gst_dbmigrate_test_%d", time.Now().UnixNano())
		createMySQLDatabase(t, mysqlDatabaseConfig(), database)
		t.Cleanup(func() {
			dropMySQLDatabase(t, mysqlDatabaseConfig(), database)
		})
		databaseConfig := mysqlDatabaseConfig()
		databaseConfig.Database = database

		plan, err := dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
			&dbmigrate.MigrateOption{})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		// A converged schema plans nothing on a re-run, custom indexes included.
		plan, err = dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.False(t, plan.Changed())
	})

	t.Run("postgres", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBPostgres, User{}, Group{}, Sample{}, DefaultedRecord{})
		require.NoError(t, err)

		database := fmt.Sprintf("gst_dbmigrate_test_%d", time.Now().UnixNano())
		adminConfig := postgresDatabaseConfig(os.Getenv(config.POSTGRES_DATABASE))
		createPostgresDatabase(t, adminConfig, database)
		t.Cleanup(func() {
			dropPostgresDatabase(t, adminConfig, database)
		})
		databaseConfig := postgresDatabaseConfig(database)

		plan, err := dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			},
		)
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOption{},
		)
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate(
			[]string{schema}, config.DBPostgres,
			databaseConfig,
			&dbmigrate.MigrateOption{
				DryRun: true,
			},
		)
		require.NoError(t, err)
		require.False(t, plan.Changed())
	})

	t.Run("sqlite", func(t *testing.T) {
		dumper, err := dbmigrate.NewSchemaDumper()
		require.NoError(t, err)
		schema, err := dumper.Dump(config.DBSqlite, User{}, Group{}, Sample{}, DefaultedRecord{})
		require.NoError(t, err)

		database := filepath.Join(t.TempDir(), "test.db")
		plan, err := dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOption{})
		require.NoError(t, err)
		require.True(t, plan.Changed())

		plan, err = dbmigrate.Migrate([]string{schema}, config.DBSqlite,
			&dbmigrate.DatabaseConfig{
				Database: database,
			},
			&dbmigrate.MigrateOption{
				DryRun: true,
			})
		require.NoError(t, err)
		require.False(t, plan.Changed())
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

	plan, err := dbmigrate.Migrate([]string{withIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 1, mysqlIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// Without EnableDrop the index survives: the plan still reports the
	// change, but renders the destructive statement as skipped instead of
	// executing it.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 1, mysqlIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// With EnableDrop the removal is planned...
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())

	// ...and applying it drops the index for good.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 0, mysqlIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// The converged schema plans nothing on a re-run.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.False(t, plan.Changed())
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

	plan, err := dbmigrate.Migrate([]string{withIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 1, postgresIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// Without EnableDrop the index survives: the plan still reports the
	// change, but renders the destructive statement as skipped instead of
	// executing it.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 1, postgresIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// With EnableDrop the removal is planned...
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())

	// ...and applying it drops the index for good.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Equal(t, 0, postgresIndexCount(t, databaseConfig, "widgets", "idx_widgets_tag"))

	// The converged schema plans nothing on a re-run.
	plan, err = dbmigrate.Migrate([]string{withoutIndex}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.False(t, plan.Changed())
}

// TestApplyRunsThePlanItWasGiven pins what a reviewer's approval covers: the
// statements they were shown. The plan is computed once, against the schema
// the database had then, and Apply runs it as it stands — it does not plan
// again against whatever the database has by the time the answer comes, which
// is a different plan for a database that moved in between.
func TestApplyRunsThePlanItWasGiven(t *testing.T) {
	database := fmt.Sprintf("gst_dbmigrate_apply_%d", time.Now().UnixNano())
	createMySQLDatabase(t, mysqlDatabaseConfig(), database)
	t.Cleanup(func() { dropMySQLDatabase(t, mysqlDatabaseConfig(), database) })
	databaseConfig := mysqlDatabaseConfig()
	databaseConfig.Database = database

	schema := "CREATE TABLE `samples` (\n" +
		"  `id` varchar(36) NOT NULL,\n" +
		"  `code` varchar(64) NOT NULL,\n" +
		"  PRIMARY KEY (`id`)\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"

	plan, err := dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Contains(t, strings.Join(plan.Statements, "\n"), "CREATE TABLE", "the plan carries the statements it showed")

	require.NoError(t, dbmigrate.Apply(plan, config.DBMySQL, databaseConfig))

	// The schema now matches, so nothing is planned any more.
	after, err := dbmigrate.Migrate([]string{schema}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.False(t, after.Changed())

	// The plan already applied is still the same statements: running it again
	// fails on the table it would create, instead of quietly planning nothing.
	// That failure is the proof the plan is not recomputed.
	require.Error(t, dbmigrate.Apply(plan, config.DBMySQL, databaseConfig))
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

	plan, err := dbmigrate.Migrate([]string{before}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Empty(t, plan.Advisory)

	// The plan for the renamed model drops `samples` and creates `records`;
	// the advisory must offer the metadata-only statements instead.
	plan, err = dbmigrate.Migrate([]string{after}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Contains(t, plan.Advisory, "RENAME TABLE `samples` TO `records`;")
	require.Contains(t, plan.Advisory, "ALTER TABLE `records` RENAME INDEX `idx_samples_code` TO `idx_records_code`;")

	// Applying the advisory instead of the plan leaves nothing to migrate.
	execMySQL(t, databaseConfig, "RENAME TABLE `samples` TO `records`")
	execMySQL(t, databaseConfig, "ALTER TABLE `records` RENAME INDEX `idx_samples_code` TO `idx_records_code`")
	plan, err = dbmigrate.Migrate([]string{after}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.False(t, plan.Changed())
	require.Empty(t, plan.Advisory)
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

	plan, err := dbmigrate.Migrate([]string{before}, config.DBMySQL, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Empty(t, plan.Advisory)

	// The model renames the table and adds a column in the same step, so the
	// created table is a column superset of the dropped one. The advisory must
	// still offer the rename and list the addition as a remaining change.
	after := strings.ReplaceAll(
		strings.Replace(before, "  `code` varchar(64) NOT NULL,\n",
			"  `code` varchar(64) NOT NULL,\n  `remark` varchar(255) NOT NULL DEFAULT '',\n", 1),
		"samples", "records",
	)
	plan, err = dbmigrate.Migrate([]string{after}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Contains(t, plan.Advisory, "RENAME TABLE `samples` TO `records`;")
	require.Contains(t, plan.Advisory, "ALTER TABLE `records` RENAME INDEX `idx_samples_code` TO `idx_records_code`;")
	require.Contains(t, plan.Advisory, "remaining change: ALTER TABLE `records` ADD COLUMN `remark`")

	// After the rename, only the remaining column addition is left in the
	// plan, and there is no drop/create pair left to advise about.
	execMySQL(t, databaseConfig, "RENAME TABLE `samples` TO `records`")
	execMySQL(t, databaseConfig, "ALTER TABLE `records` RENAME INDEX `idx_samples_code` TO `idx_records_code`")
	plan, err = dbmigrate.Migrate([]string{after}, config.DBMySQL, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Empty(t, plan.Advisory)
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

	plan, err := dbmigrate.Migrate([]string{before}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Empty(t, plan.Advisory)

	// The plan for the renamed model drops "samples" and creates "records";
	// the advisory must offer the metadata-only statements in postgres syntax.
	plan, err = dbmigrate.Migrate([]string{after}, config.DBPostgres, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Contains(t, plan.Advisory, `ALTER TABLE "samples" RENAME TO "records";`)
	require.Contains(t, plan.Advisory, `ALTER INDEX "idx_samples_code" RENAME TO "idx_records_code";`)

	// Applying the advisory instead of the plan leaves nothing to migrate.
	execPostgres(t, databaseConfig, `ALTER TABLE "samples" RENAME TO "records"`)
	execPostgres(t, databaseConfig, `ALTER INDEX "idx_samples_code" RENAME TO "idx_records_code"`)
	plan, err = dbmigrate.Migrate([]string{after}, config.DBPostgres, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.False(t, plan.Changed())
	require.Empty(t, plan.Advisory)
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

	plan, err := dbmigrate.Migrate([]string{before}, config.DBPostgres, databaseConfig, &dbmigrate.MigrateOption{})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Empty(t, plan.Advisory)

	// The model renames the table and adds a column in the same step, so the
	// created table is a column superset of the dropped one. The advisory must
	// still offer the rename and list the addition as a remaining change.
	after := strings.ReplaceAll(
		strings.Replace(before, "  code varchar(64) NOT NULL,\n",
			"  code varchar(64) NOT NULL,\n  remark varchar(255) NOT NULL DEFAULT '',\n", 1),
		"samples", "records",
	)
	plan, err = dbmigrate.Migrate([]string{after}, config.DBPostgres, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Contains(t, plan.Advisory, `ALTER TABLE "samples" RENAME TO "records";`)
	require.Contains(t, plan.Advisory, `ALTER INDEX "idx_samples_code" RENAME TO "idx_records_code";`)
	require.Contains(t, plan.Advisory, "remaining change:")
	require.Contains(t, plan.Advisory, "ADD COLUMN")

	// After the rename, only the remaining column addition is left in the
	// plan, and there is no drop/create pair left to advise about.
	execPostgres(t, databaseConfig, `ALTER TABLE "samples" RENAME TO "records"`)
	execPostgres(t, databaseConfig, `ALTER INDEX "idx_samples_code" RENAME TO "idx_records_code"`)
	plan, err = dbmigrate.Migrate([]string{after}, config.DBPostgres, databaseConfig,
		&dbmigrate.MigrateOption{DryRun: true, EnableDrop: true})
	require.NoError(t, err)
	require.True(t, plan.Changed())
	require.Empty(t, plan.Advisory)
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

	modelregistry.Base
}

func (*TagIndexedWidget) TableName() string { return "widgets" }

type PlainWidget struct {
	Tag string `json:"tag" gorm:"size:191"`

	modelregistry.Base
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
